package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Used to prepand Prometheus metrics created by this exporter.
	namespace = "rancher"
)

var (
	/**
		InfinityWorks
	 */
	agentStates   = []string{"activating", "active", "reconnecting", "disconnected", "disconnecting", "finishing-reconnect", "reconnected"}
	hostStates    = []string{"activating", "active", "deactivating", "error", "erroring", "inactive", "provisioned", "purged", "purging", "registering", "removed", "removing", "requested", "restoring", "updating_active", "updating_inactive"}
	stackStates   = []string{"activating", "active", "canceled_upgrade", "canceling_upgrade", "error", "erroring", "finishing_upgrade", "removed", "removing", "requested", "restarting", "rolling_back", "updating_active", "upgraded", "upgrading"}
	serviceStates = []string{"activating", "active", "canceled_upgrade", "canceling_upgrade", "deactivating", "finishing_upgrade", "inactive", "registering", "removed", "removing", "requested", "restarting", "rolling_back", "updating_active", "updating_inactive", "upgraded", "upgrading"}
	healthStates  = []string{"healthy", "unhealthy"}

	// health & state of host, stack, service
	hostsState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "host_state",
			Help:      "State of defined host as reported by the Rancher API",
		}, []string{"id", "name", "state"})

	hostAgentsState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "host_agent_state",
			Help:      "State of defined host agent as reported by the Rancher API",
		}, []string{"id", "name", "state"})

	stacksHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stack_health_status",
			Help:      "HealthState of defined stack as reported by Rancher",
		}, []string{"id", "name", "health_state", "system"})

	stacksState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stack_state",
			Help:      "State of defined stack as reported by Rancher",
		}, []string{"id", "name", "state", "system"})

	servicesScale = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_scale",
			Help:      "scale of defined service as reported by Rancher",
		}, []string{"name", "stack_name", "system"})

	servicesHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_health_status",
			Help:      "HealthState of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "health_state", "system"})

	servicesState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_state",
			Help:      "State of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "state", "system"})

	/**
		Extended
	 */

	// total counter of stack, service, instance
	totalStackBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stack_bootstrap_total",
		Help:      "Current total number of the started stacks in Rancher",
	}, []string{"environment_id", "environment_name", "id", "name", "system", "type"})

	totalStackFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stack_failure_total",
		Help:      "Current total number of the failure stacks in Rancher",
	}, []string{"environment_id", "environment_name", "id", "name", "system", "type"})

	totalServiceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "service_bootstrap_total",
		Help:      "Current total number of the started services in Rancher",
	}, []string{"environment_id", "environment_name", "stack_id", "stack_name", "id", "name", "system", "type"})

	totalServiceFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "service_failure_total",
		Help:      "Current total number of the failure services in Rancher",
	}, []string{"environment_id", "environment_name", "stack_id", "stack_name", "id", "name", "system", "type"})

	totalInstanceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instance_bootstrap_total",
		Help:      "Current total number of the started containers in Rancher",
	}, []string{"environment_id", "environment_name", "stack_id", "stack_name", "service_id", "service_name", "id", "name", "system", "type"})

	totalInstanceFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instance_failure_total",
		Help:      "Current total number of the failure containers in Rancher",
	}, []string{"environment_id", "environment_name", "stack_id", "stack_name", "service_id", "service_name", "id", "name", "system", "type"})

	// startup gauge
	instanceBootstrapMsCost = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "instance_startup_ms",
		Help:      "The startup milliseconds of container in Rancher",
	}, []string{"environment_id", "environment_name", "stack_id", "stack_name", "service_id", "service_name", "id", "name", "system", "type"})
)

/**
	static
 */
func newRancherClient(timeoutSeconds time.Duration) *rancherClient {
	return &rancherClient{
		&http.Client{Timeout: timeoutSeconds * time.Second},
	}
}

func newMetric() *metric {
	m := &metric{
		m:        &sync.RWMutex{},
		Projects: make(map[string]project, 10),
	}

	m.recover() // recover info

	return m
}

/**
	rancherClient class
 */
type rancherClient struct {
	client *http.Client
}

func (r *rancherClient) get(url string) *target {
	var t target
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		panic(err)
	}

	req.SetBasicAuth(cattleAccessKey, cattleSecretKey)
	resp, err := r.client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		panic(err)
	}

	return &t
}

func (r *rancherClient) post(url string, body io.Reader) (int, error) {
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return 0, err
	}

	req.SetBasicAuth(cattleAccessKey, cattleSecretKey)
	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

/**
	target class
 */
type target struct {
	Data []struct {
		HealthState    string   `json:"healthState,omitempty"`
		Key            string   `json:"key,omitempty"`
		Name           string   `json:"name,omitempty"`
		State          string   `json:"state,omitempty"`
		System         bool     `json:"system,omitempty"`
		Scale          int      `json:"scale,omitempty"`
		HostName       string   `json:"hostname,omitempty"`
		ID             string   `json:"id,omitempty"`
		StackID        string   `json:"stackId,omitempty"`
		EnvID          string   `json:"environmentId,omitempty"`
		Type           string   `json:"type,omitempty"`
		AgentState     string   `json:"agentState,omitempty"`
		CreatedTS      uint64   `json:"createdTS,omitempty"`
		FirstRunningTS uint64   `json:"firstRunningTS,omitempty"`
		ResourceData   *project `json:"resourceData,omitempty"`
	} `json:"data"`

	Pagination struct {
		Next string `json:"next,omitempty"`
	} `json:"pagination"`
}

/**
	object class
 */
type object struct {
	Id          string `json:"id,omitempty"`
	Name        string `json:"name,omitempty"`
	State       string `json:"state,omitempty"`
	HealthState string `json:"healthState,omitempty"`
	Type        string `json:"type,omitempty"`

	BootstrapCount uint64 `json:"bootstrapCount,omitempty"`
	FailureCount   uint64 `json:"failureCount,omitempty"`
}

/**
	instance class
 */
type instance struct {
	*object
	System      bool   `json:"system"`
	StartupTime uint64 `json:"startupTime,omitempty"`
	parent      *service
}

/**
	services class
 */
type service struct {
	*object
	Instances map[string]instance `json:"instances"`
	System    bool                `json:"system"`
	parent    *stack
}

func (o *service) fetch(ctx context.Context, rancherClient *rancherClient) {
	url := cattleURL + "/services/" + o.Id + "/instances?limit=100&sort=id"

	for {
		log.Infoln(">>> start fetch instance metrics on ", url)

		t := rancherClient.get(url)

		for _, d := range t.Data {
			var (
				instanceState       = d.State
				instanceId          = d.ID
				instanceName        = d.Name
				instanceSystem      = strconv.FormatBool(d.System)
				instanceType        = d.Type
				instanceStartupTime = d.FirstRunningTS - d.CreatedTS

				serviceId   = o.Id
				serviceName = o.Name

				stackId   = o.parent.Id
				stackName = o.parent.Name

				envId   = o.parent.parent.Id
				envName = o.parent.parent.Name
			)

			// Extended metrics
			if take, ok := o.Instances[instanceName]; ok {
				if take.State != instanceState {
					switch instanceState {
					case "running":
						totalInstanceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Inc()
						take.BootstrapCount += 1
					case "error":
						totalInstanceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Inc()
						take.FailureCount += 1
					}
				}

				take.Id = instanceId
				take.Type = instanceType
				take.State = instanceState
				take.System = d.System
				take.StartupTime = instanceStartupTime
			} else {
				totalInstanceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Inc()
				totalInstanceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType)

				o.Instances[instanceName] = instance{
					&object{
						Id:             instanceId,
						Name:           instanceName,
						State:          instanceState,
						Type:           instanceType,
						BootstrapCount: 1,
						FailureCount:   0,
					},
					d.System,
					instanceStartupTime,
					o,
				}
			}

			instanceBootstrapMsCost.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Set(float64(instanceStartupTime))
		}

		log.Infoln(">>> end fetch instance metrics")

		if len(t.Pagination.Next) != 0 {
			url = t.Pagination.Next
		} else {
			break
		}
	}

}

/**
	stack class
 */
type stack struct {
	*object
	Services map[string]service `json:"services"`
	System   bool               `json:"system"`
	parent   *project
}

func (o *stack) fetch(ctx context.Context, rancherClient *rancherClient) {
	url := cattleURL + "/stacks/" + o.Id + "/services?limit=100&sort=id&system=" + hideSys

	for {
		log.Infoln(">> start fetch service metrics on ", url)

		t := rancherClient.get(url)

		for _, d := range t.Data {
			var (
				serviceHealthState = d.HealthState
				serviceState       = d.State
				serviceId          = d.ID
				serviceName        = d.Name
				serviceSystem      = strconv.FormatBool(d.System)
				serviceType        = d.Type

				stackName = o.Name
				stackId   = o.Id

				envId   = o.parent.Id
				envName = o.parent.Name
			)

			// InfinityWorks metrics
			servicesScale.WithLabelValues(serviceName, stackName, serviceSystem).Set(float64(d.Scale))
			for _, y := range healthStates {
				if serviceHealthState == y {
					servicesHealth.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(1)
				} else {
					servicesHealth.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(0)
				}
			}
			for _, y := range serviceStates {
				if serviceState == y {
					servicesState.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(1)
				} else {
					servicesState.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(0)
				}
			}

			// Extended metrics
			if take, ok := o.Services[serviceName]; ok {
				if take.State != serviceState {
					switch serviceState {
					case "active":
						totalServiceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType).Inc()
						take.BootstrapCount += 1
					case "error":
						totalServiceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType).Inc()
						take.FailureCount += 1
					}
				}

				take.Id = serviceId
				take.Type = serviceType
				take.State = serviceState
				take.HealthState = serviceHealthState
				take.System = d.System
			} else {
				totalServiceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType).Inc()
				totalServiceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType)

				o.Services[serviceName] = service{
					&object{
						serviceId,
						serviceName,
						serviceState,
						serviceHealthState,
						serviceType,
						1,
						0,
					},
					make(map[string]instance, 100),
					d.System,
					o,
				}
			}
		}

		log.Infoln(">> end fetch service metrics")

		if len(t.Pagination.Next) != 0 {
			url = t.Pagination.Next
		} else {
			break
		}
	}

	wg := &sync.WaitGroup{}
	for _, d := range o.Services {
		wg.Add(1)
		go func(d *service) {
			defer wg.Done()

			d.fetch(ctx, rancherClient)
		}(&d)
	}
	wg.Wait()
}

/**
	project class
 */
type project struct {
	*object
	Stacks map[string]stack `json:"stacks"`
}

func (o *project) fetch(ctx context.Context, rancherClient *rancherClient) {
	url := cattleURL + "/projects/" + o.Id + "/stacks?limit=100&sort=id&system=" + hideSys

	for {
		log.Infoln("> start fetch stack metrics on ", url)

		t := rancherClient.get(url)

		for _, d := range t.Data {
			// InfinityWorks metrics
			var (
				stackHealthState = d.HealthState
				stackState       = d.State
				stackId          = d.ID
				stackName        = d.Name
				stackSystem      = strconv.FormatBool(d.System)
				stackType        = d.Type

				envId   = o.Id
				envName = o.Name
			)
			for _, y := range healthStates {
				if stackHealthState == y {
					stacksHealth.WithLabelValues(stackId, stackName, y, stackSystem).Set(1)
				} else {
					stacksHealth.WithLabelValues(stackId, stackName, y, stackSystem).Set(0)
				}
			}
			for _, y := range stackStates {
				if stackState == y {
					stacksState.WithLabelValues(stackId, stackName, y, stackSystem).Set(1)
				} else {
					stacksState.WithLabelValues(stackId, stackName, y, stackSystem).Set(0)
				}
			}

			// Extended metrics
			if take, ok := o.Stacks[stackName]; ok {
				if take.State != stackState {
					switch stackState {
					case "active":
						totalStackBootstrap.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType).Inc()
						take.BootstrapCount += 1
					case "error":
						totalStackFailure.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType).Inc()
						take.FailureCount += 1
					}
				}

				take.Id = stackId
				take.Type = stackType
				take.State = stackState
				take.HealthState = stackHealthState
				take.System = d.System
			} else {
				totalStackBootstrap.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType).Inc()
				totalStackFailure.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType)

				o.Stacks[stackName] = stack{
					&object{
						stackId,
						stackName,
						stackState,
						stackHealthState,
						stackType,
						1,
						0,
					},
					make(map[string]service, 100),
					d.System,
					o,
				}
			}
		}

		log.Infoln("> end fetch stack metrics")

		if len(t.Pagination.Next) != 0 {
			url = t.Pagination.Next
		} else {
			break
		}
	}

	wg := &sync.WaitGroup{}
	for _, d := range o.Stacks {
		wg.Add(1)
		go func(d *stack) {
			defer wg.Done()

			d.fetch(ctx, rancherClient)
		}(&d)
	}
	wg.Wait()

}

/**
	metric class
 */
type metric struct {
	m        *sync.RWMutex
	Projects map[string]project `json:"projects"`
}

func (o *metric) recover() {
	log.Infoln("start recover metrics")

	rancherClient := newRancherClient(scrapeTimeoutSeconds)

	t := rancherClient.get(cattleURL + "/projects")

	for _, d := range t.Data {
		var (
			envId   = d.ID
			envName = d.Name
		)

		if take, ok := o.Projects[envName]; ok {
			take.Id = envId
		} else {
			o.Projects[envName] = project{
				&object{
					Id:   envId,
					Name: envName,
				},
				make(map[string]stack, 100),
			}
		}
	}

	wg := &sync.WaitGroup{}
	for _, d := range o.Projects {
		wg.Add(1)
		go func(d project) {
			defer wg.Done()

			var (
				envId   = d.Id
				envName = d.Name
			)

			t := rancherClient.get(cattleURL + "/genericobjects?kind=rancherExporter&name=" + genObjName + "&key=" + d.Id)
			if l := len(t.Data); l != 0 {
				storeProject := t.Data[l-1].ResourceData
				for _, sStack := range storeProject.Stacks {
					sStack.parent = &d

					var (
						stackId     = sStack.Id
						stackName   = sStack.Name
						stackSystem = strconv.FormatBool(sStack.System)
						stackType   = sStack.Type
					)

					totalStackBootstrap.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType).Add(float64(sStack.BootstrapCount))
					totalStackFailure.WithLabelValues(envId, envName, stackId, stackName, stackSystem, stackType).Add(float64(sStack.FailureCount))

					for _, sService := range sStack.Services {
						sService.parent = &sStack

						var (
							serviceId     = sService.Id
							serviceName   = sService.Name
							serviceSystem = strconv.FormatBool(sService.System)
							serviceType   = sService.Type
						)

						totalServiceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType).Add(float64(sService.BootstrapCount))
						totalServiceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, serviceSystem, serviceType).Add(float64(sService.FailureCount))

						for _, sInstance := range sService.Instances {
							sInstance.parent = &sService

							var (
								instanceId     = sInstance.Id
								instanceName   = sInstance.Name
								instanceSystem = strconv.FormatBool(sInstance.System)
								instanceType   = sInstance.Type
							)

							totalInstanceBootstrap.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Add(float64(sInstance.BootstrapCount))
							totalInstanceFailure.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Add(float64(sInstance.FailureCount))

							instanceBootstrapMsCost.WithLabelValues(envId, envName, stackId, stackName, serviceId, serviceName, instanceId, instanceName, instanceSystem, instanceType).Set(float64(sInstance.StartupTime))
						}
					}

					d.Stacks[sStack.Name] = sStack
				}
			}
		}(d)
	}
	wg.Wait()

	log.Infoln("end recover metrics")
}

func (o *metric) backup() {
	o.m.RLock()
	log.Infoln("start backup metrics")

	genObjIds := make(map[string]string, 10) // key(projectId):id(genObjId)
	rancherClient := newRancherClient(0)

	// fetch again
	t := rancherClient.get(cattleURL + "/genericobjects?kind=rancherExporter&name=" + genObjName)
	for _, d := range t.Data {
		genObjIds[d.Key] = d.ID
	}

	// create 201
	//http.StatusCreated
	cCh := make(chan string)
	for _, d := range o.Projects {
		go func(url string) {
			data := make(map[string]interface{})
			data["kind"] = "rancherExporter"
			data["name"] = genObjName
			data["key"] = d.Id
			data["resourceData"] = d

			dataJson, err := json.Marshal(data)
			if err != nil {
				log.Errorf("*error created on %v", err)
				return
			}

			statusCode, err := rancherClient.post(url, bytes.NewBuffer(dataJson))
			if err != nil {
				log.Errorf("*error created on %v", err)
			} else if statusCode != http.StatusCreated {
				log.Errorln("*error created on ", url)
			} else {
				cCh <- d.Id
			}
		}(cattleURL + "/genericobjects")
	}

	// delete 202
	//http.StatusAccepted
	select {
	case projectId := <-cCh:
		url := cattleURL + "/genericobjects/" + genObjIds[projectId] + "?action=remove"

		statusCode, err := rancherClient.post(url, nil)
		if err != nil {
			log.Errorf("*error deleted on %v", err)
		} else if statusCode != http.StatusAccepted {
			log.Errorln("*error deleted on ", url)
		}
	case <-time.After(backupIntervalSeconds * time.Second):
		log.Warnf("*timeout and finish")
	}

	log.Infoln("end backup metrics")
	o.m.RUnlock()
}

func (o *metric) fetch(ctx context.Context) {
	o.m.Lock()
	log.Infoln("start fetch metrics")

	// reset InfinityWorks metrics
	hostsState.Reset()
	hostAgentsState.Reset()
	stacksHealth.Reset()
	stacksState.Reset()
	servicesScale.Reset()
	servicesHealth.Reset()
	servicesState.Reset()

	rancherClient := newRancherClient(0)
	gwg := &sync.WaitGroup{}

	// InfinityWorks metrics
	gwg.Add(1)
	go func() {
		defer gwg.Done()

		t := rancherClient.get(cattleURL + "/hosts")

		for _, d := range t.Data {
			var (
				hostName       = d.HostName
				hostState      = d.State
				hostId         = d.ID
				hostAgentState = d.AgentState
			)

			if d.Name != "" {
				hostName = d.Name
			}

			for _, y := range hostStates {
				if hostState == y {
					hostsState.WithLabelValues(hostId, hostName, y).Set(1)
				} else {
					hostsState.WithLabelValues(hostId, hostName, y).Set(0)
				}
			}

			for _, y := range agentStates {
				if hostAgentState == y {
					hostAgentsState.WithLabelValues(hostId, hostName, y).Set(1)
				} else {
					hostAgentsState.WithLabelValues(hostId, hostName, y).Set(0)
				}
			}
		}
	}()

	// Extended metrics
	gwg.Add(1)
	go func() {
		defer gwg.Done()

		t := rancherClient.get(cattleURL + "/projects")

		for _, d := range t.Data {
			var (
				envId   = d.ID
				envName = d.Name
			)

			if take, ok := o.Projects[envName]; ok {
				take.Id = envId
			} else {
				o.Projects[envName] = project{
					&object{
						Id:   envId,
						Name: envName,
					},
					make(map[string]stack, 100),
				}
			}
		}

		wg := &sync.WaitGroup{}
		for _, d := range o.Projects {
			wg.Add(1)
			go func(d *project) {
				defer wg.Done()

				d.fetch(ctx, rancherClient)
			}(&d)
		}
		wg.Wait()
	}()

	gwg.Wait()

	log.Infoln("end fetch metrics")
	o.m.Unlock()
}

func (o *metric) describe(ch chan<- *prometheus.Desc) {
	/**
		InfinityWorks
	 */
	stacksHealth.Describe(ch)
	stacksState.Describe(ch)
	servicesScale.Describe(ch)
	servicesHealth.Describe(ch)
	servicesState.Describe(ch)
	hostsState.Describe(ch)
	hostAgentsState.Describe(ch)

	/**
		Extended
	 */
	totalStackBootstrap.Describe(ch)
	totalStackFailure.Describe(ch)
	totalServiceBootstrap.Describe(ch)
	totalServiceFailure.Describe(ch)
	totalInstanceBootstrap.Describe(ch)
	totalInstanceFailure.Describe(ch)
	instanceBootstrapMsCost.Describe(ch)
}

func (o *metric) collect(ch chan<- prometheus.Metric) {
	o.m.RLock()

	/**
		InfinityWorks
	 */
	stacksHealth.Collect(ch)
	stacksState.Collect(ch)
	servicesScale.Collect(ch)
	servicesHealth.Collect(ch)
	servicesState.Collect(ch)
	hostsState.Collect(ch)
	hostAgentsState.Collect(ch)
	/**
		Extended
	 */
	totalStackBootstrap.Collect(ch)
	totalStackFailure.Collect(ch)
	totalServiceBootstrap.Collect(ch)
	totalServiceFailure.Collect(ch)
	totalInstanceBootstrap.Collect(ch)
	totalInstanceFailure.Collect(ch)
	instanceBootstrapMsCost.Collect(ch)

	o.m.RUnlock()
}
