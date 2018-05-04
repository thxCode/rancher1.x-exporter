package main

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	logger "github.com/Sirupsen/logrus"
	"github.com/buger/jsonparser"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// Used to prepand Prometheus metrics created by this exporter.
	namespace  = "rancher"
	specialTag = "__rancher__"
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
	infinityWorksHostsState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "host_state",
			Help:      "State of defined host as reported by the Rancher API",
		}, []string{"id", "name", "state"})

	infinityWorksHostAgentsState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "host_agent_state",
			Help:      "State of defined host agent as reported by the Rancher API",
		}, []string{"id", "name", "state"})

	infinityWorksStacksHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stack_health_status",
			Help:      "HealthState of defined stack as reported by Rancher",
		}, []string{"id", "name", "health_state", "system"})

	infinityWorksStacksState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "stack_state",
			Help:      "State of defined stack as reported by Rancher",
		}, []string{"id", "name", "state", "system"})

	infinityWorksServicesScale = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_scale",
			Help:      "scale of defined service as reported by Rancher",
		}, []string{"name", "stack_name", "system"})

	infinityWorksServicesHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_health_status",
			Help:      "HealthState of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "health_state", "system"})

	infinityWorksServicesState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_state",
			Help:      "State of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "state", "system"})

	/**
		Extended
	 */

	// total counter of stack, service, instance

	extendingTotalStackInitializations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_initialization_total",
		Help:      "Current total number of the initialization stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalSuccessStackInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_initialization_success_total",
		Help:      "Current total number of the healthy and active initialization stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalErrorStackInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_initialization_error_total",
		Help:      "Current total number of the unhealthy or error initialization stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalServiceInitializations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_initialization_total",
		Help:      "Current total number of the initialization services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalSuccessServiceInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_initialization_success_total",
		Help:      "Current total number of the healthy and active initialization services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalErrorServiceInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_initialization_error_total",
		Help:      "Current total number of the unhealthy or error initialization services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalInstanceInitializations = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_initialization_total",
		Help:      "Current total number of the initialization instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	extendingTotalSuccessInstanceInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_initialization_success_total",
		Help:      "Current total number of the healthy and active initialization instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	extendingTotalErrorInstanceInitialization = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_initialization_error_total",
		Help:      "Current total number of the unhealthy or error initialization instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	extendingTotalStackBootstraps = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_bootstrap_total",
		Help:      "Current total number of the bootstrap stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalSuccessStackBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_bootstrap_success_total",
		Help:      "Current total number of the healthy and active bootstrap stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalErrorStackBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stacks_bootstrap_error_total",
		Help:      "Current total number of the unhealthy or error bootstrap stacks in Rancher",
	}, []string{"environment_name", "name"})

	extendingTotalServiceBootstraps = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_bootstrap_total",
		Help:      "Current total number of the bootstrap services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalSuccessServiceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_bootstrap_success_total",
		Help:      "Current total number of the healthy and active bootstrap services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalErrorServiceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "services_bootstrap_error_total",
		Help:      "Current total number of the unhealthy or error bootstrap services in Rancher",
	}, []string{"environment_name", "stack_name", "name"})

	extendingTotalInstanceBootstraps = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_bootstrap_total",
		Help:      "Current total number of the bootstrap instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	extendingTotalSuccessInstanceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_bootstrap_success_total",
		Help:      "Current total number of the healthy and active bootstrap instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	extendingTotalErrorInstanceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instances_bootstrap_error_total",
		Help:      "Current total number of the unhealthy or error bootstrap instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name"})

	// startup gauge
	extendingInstanceBootstrapMsCost = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "instance_bootstrap_ms",
		Help:      "The bootstrap milliseconds of instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name", "system", "type"})

	// heartbeat
	extendingStackHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "stack_heartbeat",
		Help:      "The heartbeat of stacks in Rancher",
	}, []string{"environment_name", "name", "system", "type"})

	extendingServiceHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "service_heartbeat",
		Help:      "The heartbeat of services in Rancher",
	}, []string{"environment_name", "stack_name", "name", "system", "type"})

	extendingInstanceHeartbeat = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "instance_heartbeat",
		Help:      "The heartbeat of instances in Rancher",
	}, []string{"environment_name", "stack_name", "service_name", "name", "system", "type"})
)

type httpClient struct {
	client *http.Client
}

func (r *httpClient) get(url string) ([]byte, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.SetBasicAuth(cattleAccessKey, cattleSecretKey)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if bs, err := ioutil.ReadAll(resp.Body); err != nil {
		return nil, err
	} else {
		return bs, nil
	}
}

func newHttpClient(timeoutSeconds time.Duration) *httpClient {
	return &httpClient{
		&http.Client{Timeout: timeoutSeconds},
	}
}

type buffMsg struct {
	id            string
	parentId      string
	name          string
	state         string
	healthState   string
	transitioning string
	serviceName   string
	stackName     string
}

/**
	RancherExporter
 */
type rancherExporter struct {
	projectId     string
	projectName   string
	mutex         *sync.Mutex
	websocketConn *websocket.Conn

	stacksBuff    chan buffMsg
	servicesBuff  chan buffMsg
	instancesBuff chan buffMsg

	recreateWebsocket func() *websocket.Conn
}

func (r *rancherExporter) Describe(ch chan<- *prometheus.Desc) {
	infinityWorksStacksHealth.Describe(ch)
	infinityWorksStacksState.Describe(ch)
	infinityWorksServicesScale.Describe(ch)
	infinityWorksServicesHealth.Describe(ch)
	infinityWorksServicesState.Describe(ch)
	infinityWorksHostsState.Describe(ch)
	infinityWorksHostAgentsState.Describe(ch)

	extendingTotalStackInitializations.Describe(ch)
	extendingTotalSuccessStackInitialization.Describe(ch)
	extendingTotalErrorStackInitialization.Describe(ch)
	extendingTotalServiceInitializations.Describe(ch)
	extendingTotalSuccessServiceInitialization.Describe(ch)
	extendingTotalErrorServiceInitialization.Describe(ch)
	extendingTotalInstanceInitializations.Describe(ch)
	extendingTotalSuccessInstanceInitialization.Describe(ch)
	extendingTotalErrorInstanceInitialization.Describe(ch)

	extendingTotalStackBootstraps.Describe(ch)
	extendingTotalSuccessStackBootstrap.Describe(ch)
	extendingTotalErrorStackBootstrap.Describe(ch)
	extendingTotalServiceBootstraps.Describe(ch)
	extendingTotalSuccessServiceBootstrap.Describe(ch)
	extendingTotalErrorServiceBootstrap.Describe(ch)
	extendingTotalInstanceBootstraps.Describe(ch)
	extendingTotalSuccessInstanceBootstrap.Describe(ch)
	extendingTotalErrorInstanceBootstrap.Describe(ch)
	extendingInstanceBootstrapMsCost.Describe(ch)

	extendingInstanceHeartbeat.Describe(ch)
	extendingServiceHeartbeat.Describe(ch)
	extendingStackHeartbeat.Describe(ch)
}

func (r *rancherExporter) Collect(ch chan<- prometheus.Metric) {
	r.asyncMetrics(ch)

	r.syncMetrics(ch)
}

func (r *rancherExporter) Stop() {
	if r.websocketConn != nil {
		r.websocketConn.Close()
	}

	close(r.instancesBuff)
	close(r.servicesBuff)
	close(r.stacksBuff)
}

func (r *rancherExporter) asyncMetrics(ch chan<- prometheus.Metric) {
	// collect
	extendingTotalStackBootstraps.Collect(ch)
	extendingTotalSuccessStackBootstrap.Collect(ch)
	extendingTotalErrorStackBootstrap.Collect(ch)
	extendingTotalStackInitializations.Collect(ch)
	extendingTotalSuccessStackInitialization.Collect(ch)
	extendingTotalErrorStackInitialization.Collect(ch)

	extendingTotalServiceBootstraps.Collect(ch)
	extendingTotalSuccessServiceBootstrap.Collect(ch)
	extendingTotalErrorServiceBootstrap.Collect(ch)
	extendingTotalServiceInitializations.Collect(ch)
	extendingTotalSuccessServiceInitialization.Collect(ch)
	extendingTotalErrorServiceInitialization.Collect(ch)

	extendingTotalInstanceBootstraps.Collect(ch)
	extendingTotalSuccessInstanceBootstrap.Collect(ch)
	extendingTotalErrorInstanceBootstrap.Collect(ch)
	extendingTotalInstanceInitializations.Collect(ch)
	extendingTotalSuccessInstanceInitialization.Collect(ch)
	extendingTotalErrorInstanceInitialization.Collect(ch)

	extendingInstanceBootstrapMsCost.Collect(ch)
}

func (r *rancherExporter) syncMetrics(ch chan<- prometheus.Metric) {
	projectId := r.projectId
	projectName := r.projectName

	defer func() {
		if err := recover(); err != nil {
			logger.Errorln(err)
		}
	}()

	r.mutex.Lock()
	defer r.mutex.Unlock()

	infinityWorksHostsState.Reset()
	infinityWorksHostAgentsState.Reset()
	infinityWorksStacksHealth.Reset()
	infinityWorksStacksState.Reset()
	extendingStackHeartbeat.Reset()
	infinityWorksServicesScale.Reset()
	infinityWorksServicesHealth.Reset()
	infinityWorksServicesState.Reset()
	extendingServiceHeartbeat.Reset()
	extendingInstanceHeartbeat.Reset()

	hc := newHttpClient(60 * time.Second)
	gwg := &sync.WaitGroup{}

	gwg.Add(1)
	go func() {
		defer gwg.Done()

		if hostsRespBytes, err := hc.get(cattleURL + "/hosts"); err != nil {
			logger.Warnln(err)
		} else {
			jsonparser.ArrayEach(hostsRespBytes, func(hostBytes []byte, dataType jsonparser.ValueType, offset int, err error) {
				hostName, _ := jsonparser.GetString(hostBytes, "name")
				hostState, _ := jsonparser.GetString(hostBytes, "state")
				hostId, _ := jsonparser.GetString(hostBytes, "id")
				hostAgentState, _ := jsonparser.GetString(hostBytes, "agentState")

				if len(hostName) == 0 {
					hostName, _ = jsonparser.GetString(hostBytes, "hostname")
				}

				for _, y := range hostStates {
					if hostState == y {
						infinityWorksHostsState.WithLabelValues(hostId, hostName, y).Set(1)
					} else {
						infinityWorksHostsState.WithLabelValues(hostId, hostName, y).Set(0)
					}
				}

				for _, y := range agentStates {
					if hostAgentState == y {
						infinityWorksHostAgentsState.WithLabelValues(hostId, hostName, y).Set(1)
					} else {
						infinityWorksHostAgentsState.WithLabelValues(hostId, hostName, y).Set(0)
					}
				}

			}, "data")
		}
	}()

	gwg.Add(1)
	go func() {
		defer gwg.Done()

		stacksAddress := cattleURL + "/projects/" + projectId + "/stacks?limit=100&sort=id"
		if hideSys {
			stacksAddress += "&system=false"
		}

		stkwg := &sync.WaitGroup{}
		for {
			if stacksRespBytes, err := hc.get(stacksAddress); err != nil {
				logger.Errorln(stacksAddress, err)
				break
			} else {
				jsonparser.ArrayEach(stacksRespBytes, func(stackBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

					stkwg.Add(1)
					go func() {
						defer stkwg.Done()

						stackId, _ := jsonparser.GetString(stackBytes, "id")
						stackName, _ := jsonparser.GetString(stackBytes, "name")
						stackSystem, _ := jsonparser.GetUnsafeString(stackBytes, "system")
						stackType, _ := jsonparser.GetString(stackBytes, "type")
						stackHealthState, _ := jsonparser.GetString(stackBytes, "healthState")
						stackState, _ := jsonparser.GetString(stackBytes, "state")

						for _, y := range healthStates {
							if stackHealthState == y {
								infinityWorksStacksHealth.WithLabelValues(stackId, stackName, y, stackSystem).Set(1)
							} else {
								infinityWorksStacksHealth.WithLabelValues(stackId, stackName, y, stackSystem).Set(0)
							}
						}

						for _, y := range stackStates {
							if stackState == y {
								infinityWorksStacksState.WithLabelValues(stackId, stackName, y, stackSystem).Set(1)
							} else {
								infinityWorksStacksState.WithLabelValues(stackId, stackName, y, stackSystem).Set(0)
							}
						}

						extendingStackHeartbeat.WithLabelValues(projectName, stackName, stackSystem, stackType).Set(float64(1))

						servicesAddress := cattleURL + "/stacks/" + stackId + "/services?limit=100&sort=id"
						if hideSys {
							servicesAddress += "&system=false"
						}

						svcwg := &sync.WaitGroup{}
						for {
							if servicesRespBytes, err := hc.get(servicesAddress); err != nil {
								logger.Errorln(servicesAddress, err)
								break
							} else {
								jsonparser.ArrayEach(servicesRespBytes, func(serviceBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

									svcwg.Add(1)
									go func() {
										defer svcwg.Done()

										serviceId, _ := jsonparser.GetString(serviceBytes, "id")
										serviceName, _ := jsonparser.GetString(serviceBytes, "name")
										serviceSystem, _ := jsonparser.GetUnsafeString(serviceBytes, "system")
										serviceType, _ := jsonparser.GetString(serviceBytes, "type")
										serviceHealthState, _ := jsonparser.GetString(serviceBytes, "healthState")
										serviceState, _ := jsonparser.GetString(serviceBytes, "state")
										serviceScale, _ := jsonparser.GetInt(serviceBytes, "scale")

										infinityWorksServicesScale.WithLabelValues(serviceName, stackName, serviceSystem).Set(float64(serviceScale))
										for _, y := range healthStates {
											if serviceHealthState == y {
												infinityWorksServicesHealth.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(1)
											} else {
												infinityWorksServicesHealth.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(0)
											}
										}

										for _, y := range serviceStates {
											if serviceState == y {
												infinityWorksServicesState.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(1)
											} else {
												infinityWorksServicesState.WithLabelValues(serviceId, stackId, serviceName, stackName, y, serviceSystem).Set(0)
											}
										}

										extendingServiceHeartbeat.WithLabelValues(projectName, stackName, serviceName, serviceSystem, serviceType).Set(float64(1))

										instancesAddress := cattleURL + "/services/" + serviceId + "/instances?limit=100&sort=id"
										if hideSys {
											instancesAddress += "&system=false"
										}

										for {
											if instancesRespBytes, err := hc.get(instancesAddress); err != nil {
												logger.Errorln(instancesAddress, err)
												break
											} else {
												jsonparser.ArrayEach(instancesRespBytes, func(instanceBytes []byte, dataType jsonparser.ValueType, offset int, err error) {
													instanceName, _ := jsonparser.GetString(instanceBytes, "name")
													instanceSystem, _ := jsonparser.GetUnsafeString(instanceBytes, "system")
													instanceType, _ := jsonparser.GetString(instanceBytes, "type")

													extendingInstanceHeartbeat.WithLabelValues(projectName, stackName, serviceName, instanceName, instanceSystem, instanceType).Set(float64(1))

													if instanceFirstRunningTS, _ := jsonparser.GetInt(instanceBytes, "firstRunningTS"); instanceFirstRunningTS != 0 {
														instanceCreatedTS, _ := jsonparser.GetInt(instanceBytes, "createdTS")
														extendingInstanceBootstrapMsCost.WithLabelValues(projectName, stackName, serviceName, instanceName, instanceSystem, instanceType).Set(float64(instanceFirstRunningTS - instanceCreatedTS))
													}

												}, "data")

												if next, _ := jsonparser.GetString(instancesRespBytes, "pagination", "next"); len(next) == 0 {
													break
												} else {
													instancesAddress = next
												}

											}

										}

									}()

								}, "data")

								if next, _ := jsonparser.GetString(servicesRespBytes, "pagination", "next"); len(next) == 0 {
									break
								} else {
									servicesAddress = next
								}
							}

						}
						svcwg.Wait()

					}()

				}, "data")

				if next, _ := jsonparser.GetString(stacksRespBytes, "pagination", "next"); len(next) == 0 {
					break
				} else {
					stacksAddress = next
				}
			}
		}
		stkwg.Wait()

	}()

	gwg.Wait()

	// collect
	infinityWorksHostsState.Collect(ch)
	infinityWorksHostAgentsState.Collect(ch)
	infinityWorksStacksHealth.Collect(ch)
	infinityWorksStacksState.Collect(ch)
	extendingStackHeartbeat.Collect(ch)
	infinityWorksServicesScale.Collect(ch)
	infinityWorksServicesHealth.Collect(ch)
	infinityWorksServicesState.Collect(ch)
	extendingServiceHeartbeat.Collect(ch)
	extendingInstanceHeartbeat.Collect(ch)

}

func (r *rancherExporter) collectingExtending() {
	projectId := r.projectId
	projectName := r.projectName

	stackIdNameMap := &sync.Map{}

	hc := newHttpClient(30 * time.Second)
	stacksAddress := cattleURL + "/projects/" + projectId + "/stacks?limit=100&sort=id"
	if hideSys {
		stacksAddress += "&system=false"
	}

	stkwg := &sync.WaitGroup{}
	for {
		if stacksRespBytes, err := hc.get(stacksAddress); err != nil {
			logger.Errorln(stacksAddress, err)
			break
		} else {
			jsonparser.ArrayEach(stacksRespBytes, func(stackBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

				stkwg.Add(1)
				go func() {
					defer stkwg.Done()

					stackId, _ := jsonparser.GetString(stackBytes, "id")
					stackName, _ := jsonparser.GetString(stackBytes, "name")
					stackHealthState, _ := jsonparser.GetString(stackBytes, "healthState")
					stackState, _ := jsonparser.GetString(stackBytes, "state")

					stackIdNameMap.Store(stackId, stackName)

					// init bootstrap
					extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag)
					extendingTotalStackBootstraps.WithLabelValues(projectName, stackName)
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName)

					switch stackState {
					case "active":
						if stackHealthState == "unhealthy" {
							extendingTotalStackInitializations.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalStackInitializations.WithLabelValues(projectName, stackName).Inc()
							extendingTotalSuccessStackInitialization.WithLabelValues(projectName, specialTag)
							extendingTotalSuccessStackInitialization.WithLabelValues(projectName, stackName)
							extendingTotalErrorStackInitialization.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalErrorStackInitialization.WithLabelValues(projectName, stackName).Inc()
						} else if stackHealthState == "healthy" {
							extendingTotalStackInitializations.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalStackInitializations.WithLabelValues(projectName, stackName).Inc()
							extendingTotalSuccessStackInitialization.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalSuccessStackInitialization.WithLabelValues(projectName, stackName).Inc()
							extendingTotalErrorStackInitialization.WithLabelValues(projectName, specialTag)
							extendingTotalErrorStackInitialization.WithLabelValues(projectName, stackName)
						}
					case "error":
						extendingTotalStackInitializations.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalStackInitializations.WithLabelValues(projectName, stackName).Inc()
						extendingTotalSuccessStackInitialization.WithLabelValues(projectName, specialTag)
						extendingTotalSuccessStackInitialization.WithLabelValues(projectName, stackName)
						extendingTotalErrorStackInitialization.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalErrorStackInitialization.WithLabelValues(projectName, stackName).Inc()
					}

					servicesAddress := cattleURL + "/stacks/" + stackId + "/services?limit=100&sort=id"
					if hideSys {
						servicesAddress += "&system=false"
					}

					svcwg := &sync.WaitGroup{}
					for {
						if servicesRespBytes, err := hc.get(servicesAddress); err != nil {
							logger.Errorln(servicesAddress, err)
							break
						} else {
							jsonparser.ArrayEach(servicesRespBytes, func(serviceBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

								svcwg.Add(1)
								go func() {
									defer svcwg.Done()

									serviceId, _ := jsonparser.GetString(serviceBytes, "id")
									serviceName, _ := jsonparser.GetString(serviceBytes, "name")
									serviceHealthState, _ := jsonparser.GetString(serviceBytes, "healthState")
									serviceState, _ := jsonparser.GetString(serviceBytes, "state")

									extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag)
									extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag)
									extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceName)
									extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
									extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
									extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceName)
									extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
									extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
									extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceName)

									switch serviceState {
									case "active":
										extendingTotalServiceInitializations.WithLabelValues(projectName, specialTag, specialTag).Inc()
										extendingTotalServiceInitializations.WithLabelValues(projectName, stackName, specialTag).Inc()
										extendingTotalServiceInitializations.WithLabelValues(projectName, stackName, serviceName).Inc()

										if serviceHealthState == "unhealthy" {
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, specialTag, specialTag)
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, specialTag)
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, serviceName)
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, specialTag, specialTag).Inc()
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, specialTag).Inc()
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, serviceName).Inc()
										} else if serviceHealthState == "healthy" {
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, specialTag, specialTag).Inc()
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, specialTag).Inc()
											extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, serviceName).Inc()
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, specialTag, specialTag)
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, specialTag)
											extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, serviceName)
										}
									case "error":
										extendingTotalServiceInitializations.WithLabelValues(projectName, specialTag, specialTag).Inc()
										extendingTotalServiceInitializations.WithLabelValues(projectName, stackName, specialTag).Inc()
										extendingTotalServiceInitializations.WithLabelValues(projectName, stackName, serviceName).Inc()
										extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, specialTag, specialTag)
										extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, specialTag)
										extendingTotalSuccessServiceInitialization.WithLabelValues(projectName, stackName, serviceName)
										extendingTotalErrorServiceInitialization.WithLabelValues(projectName, specialTag, specialTag).Inc()
										extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, specialTag).Inc()
										extendingTotalErrorServiceInitialization.WithLabelValues(projectName, stackName, serviceName).Inc()
									}

									instancesAddress := cattleURL + "/services/" + serviceId + "/instances?limit=100&sort=id"
									if hideSys {
										instancesAddress += "&system=false"
									}

									for {
										if instancesRespBytes, err := hc.get(instancesAddress); err != nil {
											logger.Errorln(instancesAddress, err)
											break
										} else {
											jsonparser.ArrayEach(instancesRespBytes, func(instanceBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

												instanceName, _ := jsonparser.GetString(instanceBytes, "name")
												instanceSystem, _ := jsonparser.GetUnsafeString(instanceBytes, "system")
												instanceType, _ := jsonparser.GetString(instanceBytes, "type")
												instanceState, _ := jsonparser.GetString(instanceBytes, "state")
												instanceFirstRunningTS, _ := jsonparser.GetInt(instanceBytes, "firstRunningTS")
												instanceCreatedTS, _ := jsonparser.GetInt(instanceBytes, "createdTS")

												extendingTotalInstanceBootstraps.WithLabelValues(projectName, specialTag, specialTag, specialTag)
												extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, specialTag, specialTag)
												extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, serviceName, specialTag)
												extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, serviceName, instanceName)
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, specialTag, specialTag)
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, specialTag)
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, instanceName)
												extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
												extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, stackName, specialTag, specialTag)
												extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, specialTag)
												extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, instanceName)

												switch instanceState {
												case "stopped":
													fallthrough
												case "running":
													extendingTotalInstanceInitializations.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
													extendingTotalInstanceInitializations.WithLabelValues(projectName, stackName, specialTag, specialTag).Inc()
													extendingTotalInstanceInitializations.WithLabelValues(projectName, stackName, serviceName, specialTag).Inc()
													extendingTotalInstanceInitializations.WithLabelValues(projectName, stackName, serviceName, instanceName).Inc()
													extendingTotalSuccessInstanceInitialization.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
													extendingTotalSuccessInstanceInitialization.WithLabelValues(projectName, stackName, specialTag, specialTag).Inc()
													extendingTotalSuccessInstanceInitialization.WithLabelValues(projectName, stackName, serviceName, specialTag).Inc()
													extendingTotalSuccessInstanceInitialization.WithLabelValues(projectName, stackName, serviceName, instanceName).Inc()
													extendingTotalErrorInstanceInitialization.WithLabelValues(projectName, specialTag, specialTag, specialTag)
													extendingTotalErrorInstanceInitialization.WithLabelValues(projectName, stackName, specialTag, specialTag)
													extendingTotalErrorInstanceInitialization.WithLabelValues(projectName, stackName, serviceName, specialTag)
													extendingTotalErrorInstanceInitialization.WithLabelValues(projectName, stackName, serviceName, instanceName)

													if instanceFirstRunningTS != 0 {
														instanceStartupTime := instanceFirstRunningTS - instanceCreatedTS
														extendingInstanceBootstrapMsCost.WithLabelValues(projectName, stackName, serviceName, instanceName, instanceSystem, instanceType).Set(float64(instanceStartupTime))
													}
												}

											}, "data")

											if next, _ := jsonparser.GetString(instancesRespBytes, "pagination", "next"); len(next) == 0 {
												break
											} else {
												instancesAddress = next
											}

										}

									}

								}()

							}, "data")

							if next, _ := jsonparser.GetString(servicesRespBytes, "pagination", "next"); len(next) == 0 {
								break
							} else {
								servicesAddress = next
							}
						}

					}
					svcwg.Wait()

				}()

			}, "data")

			if next, _ := jsonparser.GetString(stacksRespBytes, "pagination", "next"); len(next) == 0 {
				break
			} else {
				stacksAddress = next
			}
		}
	}
	stkwg.Wait()

	// event watcher
	go func() {
		for {
		recall:
			_, messageBytes, err := r.websocketConn.ReadMessage()
			if err != nil {
				logger.Warnln("reconnect websocket")
				r.websocketConn = r.recreateWebsocket()
				goto recall
			}

			if resourceType, _ := jsonparser.GetString(messageBytes, "resourceType"); len(resourceType) != 0 {
				resourceBytes, _, _, err := jsonparser.Get(messageBytes, "data", "resource")
				if err != nil {
					logger.Warnln(err)
					continue
				}

				baseType, _ := jsonparser.GetString(resourceBytes, "baseType")
				switch baseType {
				case "stack":
					id, _ := jsonparser.GetString(resourceBytes, "id")
					name, _ := jsonparser.GetString(resourceBytes, "name")
					state, _ := jsonparser.GetString(resourceBytes, "state")
					healthState, _ := jsonparser.GetString(resourceBytes, "healthState")
					transitioning, _ := jsonparser.GetString(resourceBytes, "transitioning")

					stackIdNameMap.LoadOrStore(id, name)

					r.stacksBuff <- buffMsg{
						id:            id,
						name:          name,
						state:         state,
						healthState:   healthState,
						transitioning: transitioning,
					}
				case "service":
					id, _ := jsonparser.GetString(resourceBytes, "id")
					stackId, _ := jsonparser.GetString(resourceBytes, "stackId")
					name, _ := jsonparser.GetString(resourceBytes, "name")
					state, _ := jsonparser.GetString(resourceBytes, "state")
					healthState, _ := jsonparser.GetString(resourceBytes, "healthState")
					transitioning, _ := jsonparser.GetString(resourceBytes, "transitioning")

					var stackName string
					if val, ok := stackIdNameMap.Load(stackId); ok {
						stackName = val.(string)
					} else if stackLink, err := jsonparser.GetString(resourceBytes, "links", "stack"); err == nil {
						hc := newHttpClient(10 * time.Second)
						if stackRespBytes, err := hc.get(stackLink); err == nil {
							stackName, _ = jsonparser.GetString(stackRespBytes, "name")
							stackIdNameMap.LoadOrStore(stackId, stackName)
						}
					}

					r.servicesBuff <- buffMsg{
						id:            id,
						parentId:      stackId,
						name:          name,
						state:         state,
						healthState:   healthState,
						transitioning: transitioning,
						stackName:     stackName,
					}
				case "instance":
					id, _ := jsonparser.GetString(resourceBytes, "id")
					name, _ := jsonparser.GetString(resourceBytes, "name")
					state, _ := jsonparser.GetString(resourceBytes, "state")
					healthState, _ := jsonparser.GetString(resourceBytes, "healthState")
					transitioning, _ := jsonparser.GetString(resourceBytes, "transitioning")
					labelStackServiceName, _ := jsonparser.GetString(resourceBytes, "labels", "io.rancher.stack_service.name")
					serviceId, _ := jsonparser.GetString(resourceBytes, "serviceIds", "[0]")

					labelStackServiceNameSplit := strings.Split(labelStackServiceName, "/")

					r.instancesBuff <- buffMsg{
						id:            id,
						parentId:      serviceId,
						name:          name,
						state:         state,
						healthState:   healthState,
						transitioning: transitioning,
						stackName:     labelStackServiceNameSplit[0],
						serviceName:   labelStackServiceNameSplit[1],
					}
				}
			}
		}

	}()

	// stack event handler
	go func() {
		activatingStackLoop := &sync.Map{}

		const (
			activating int64 = iota
			active
			inactive
		)

		stackIdNameMap.Range(func(key, value interface{}) bool {
			count := active
			activatingStackLoop.Store(value.(string), &count)
			return true
		})

		for stackMsg := range r.stacksBuff {
			logger.Debugf("[[stack   ]]: %+v", stackMsg)

			switch stackMsg.state {
			case "activating":
				_, exist := activatingStackLoop.Load(stackMsg.name)
				if !exist {
					extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
					extendingTotalStackBootstraps.WithLabelValues(projectName, stackMsg.name).Inc()
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackMsg.name)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name)

					logger.Infoln("stack [", stackMsg.name, "] bs count + 1")
					count := activating
					activatingStackLoop.Store(stackMsg.name, &count)
				}
			case "active":
				countPtr, exist := activatingStackLoop.Load(stackMsg.name)
				if !exist {
					count := activating
					countPtr = &count
					activatingStackLoop.Store(stackMsg.name, countPtr)

					extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
					extendingTotalStackBootstraps.WithLabelValues(projectName, stackMsg.name).Inc()
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackMsg.name)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag)
					extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name)

					logger.Infoln("stack [", stackMsg.name, "] bs count + 1")
				}

				if stackMsg.healthState == "unhealthy" {
					// if one service isn't degraded
					isSomeServiceDegraded := false
					hc := newHttpClient(10 * time.Second)
					if servicesRespBytes, err := hc.get(cattleURL + "/stacks/" + stackMsg.id + "/services"); err == nil {
						jsonparser.ArrayEach(servicesRespBytes, func(serviceRespBytes []byte, dataType jsonparser.ValueType, offset int, err error) {
							healthState, _ := jsonparser.GetString(serviceRespBytes, "healthState")
							if healthState == "degraded" {
								isSomeServiceDegraded = true
								// todo break
							}
						}, "data")
					}

					if !isSomeServiceDegraded {
						atomic.CompareAndSwapInt64(countPtr.(*int64), active, inactive)
					}
				} else if stackMsg.healthState == "healthy" {
					if atomic.CompareAndSwapInt64(countPtr.(*int64), inactive, activating) {
						extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalStackBootstraps.WithLabelValues(projectName, stackMsg.name).Inc()

						logger.Infoln("stack [", stackMsg.name, "] bs count + 1")
					}
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), activating, active) {
					if stackMsg.healthState == "healthy" {
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

						logger.Infoln("stack [", stackMsg.name, "] bs success + 1")
					} else if stackMsg.healthState == "unhealthy" {
						extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

						logger.Infoln("stack [", stackMsg.name, "] bs error + 1")
					}
				}
			case "error":
				extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
				extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

				logger.Infoln("stack [", stackMsg.name, "] bs error + 1")
			case "removed":
				stackIdNameMap.Delete(stackMsg.id)
				activatingStackLoop.Delete(stackMsg.name)
			}
		}
	}()

	// service event handler
	go func() {
		activatingServicesLoop := &sync.Map{}

		const (
			stopped         int64 = iota
			upgrading
			upgraded
			rollingBack
			canceledUpgrade
			activating
			restarting
			updatingActive
			active
		)

		for serviceMsg := range r.servicesBuff {
			logger.Debugf("[[service ]]: %+v", serviceMsg)

			stackName := serviceMsg.stackName
			loopKey := stackName + "-" + serviceMsg.name

			switch serviceMsg.state {
			case "activating":
				_, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)

					logger.Infoln("service [", serviceMsg.name, "] bs count + 1")
					count := activating
					activatingServicesLoop.Store(loopKey, &count)

					continue
				}
			case "upgrading":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					count := active
					countPtr = &count
					activatingServicesLoop.Store(loopKey, countPtr)
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), active, upgrading) {
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)

					logger.Infoln("service [", serviceMsg.name, "] bs count + 1")
				}
			case "restarting":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					count := active
					countPtr = &count
					activatingServicesLoop.Store(loopKey, countPtr)
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), active, restarting) {
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag)
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name)

					logger.Infoln("service [", serviceMsg.name, "] bs count + 1")
				}
			case "canceled-upgrade":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					continue
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), upgrading, canceledUpgrade) {
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					logger.Infoln("service [", serviceMsg.name, "] bs error + 1")
				}
			case "upgraded":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					continue
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), upgrading, upgraded) {

					// for 1 service of 1 stack
					if len(serviceMsg.parentId) != 0 {
						hc := newHttpClient(10 * time.Second)
						if servicesRespBytes, err := hc.get(cattleURL + "/stacks/" + serviceMsg.parentId + "/services"); err == nil {
							_, _, _, err := jsonparser.Get(servicesRespBytes, "data", "[1]")
							if err == jsonparser.KeyPathNotFoundError {

								extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstraps.WithLabelValues(projectName, stackName).Inc()

								logger.Infoln("stack [", stackName, "] bs count + 1")

								if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" {
									extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
									extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName).Inc()

									logger.Infoln("stack [", stackName, "] bs success + 1")
								} else if serviceMsg.healthState == "unhealthy" {
									extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
									extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName).Inc()

									logger.Infoln("stack [", stackName, "] bs error + 1")
								}
							}
						}
					}

					if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" { // healthy start
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

						logger.Infoln("service [", serviceMsg.name, "] bs success + 1")
					} else if serviceMsg.healthState == "unhealthy" { // unhealthy start
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

						logger.Infoln("service [", serviceMsg.name, "] bs error + 1")
					}
				}
			case "rolling-back":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					continue
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), upgrading, canceledUpgrade) {
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					logger.Infoln("service [", serviceMsg.name, "] bs error + 1")
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), upgraded, rollingBack) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), canceledUpgrade, rollingBack) {

					// for 1 service of 1 stack
					if len(serviceMsg.parentId) != 0 {
						hc := newHttpClient(10 * time.Second)
						if servicesRespBytes, err := hc.get(cattleURL + "/stacks/" + serviceMsg.parentId + "/services"); err == nil {
							_, _, _, err := jsonparser.Get(servicesRespBytes, "data", "[1]")
							if err == jsonparser.KeyPathNotFoundError {

								extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstraps.WithLabelValues(projectName, stackName).Inc()

								logger.Infoln("stack [", stackName, "] bs count + 1")

								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName).Inc()

								logger.Infoln("stack [", stackName, "] bs success + 1")

							}
						}
					}

					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					logger.Infoln("service [", serviceMsg.name, "] bs count + 1")

				}
			case "updating-active":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					count := active
					countPtr = &count
					activatingServicesLoop.Store(loopKey, countPtr)
				}

				// if instances length only one then
				if len(serviceMsg.id) != 0 {
					hc := newHttpClient(10 * time.Second)
					if instancesRespBytes, err := hc.get(cattleURL + "/services/" + serviceMsg.id + "/instances"); err == nil {
						_, _, _, err := jsonparser.Get(instancesRespBytes, "data", "[1]")
						if err == jsonparser.KeyPathNotFoundError {
							if atomic.CompareAndSwapInt64(countPtr.(*int64), active, updatingActive) {
								extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
								extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
								extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

								logger.Infoln("service [", serviceMsg.name, "] bs count + 1")

								// at the same time, stack must be count
								extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstraps.WithLabelValues(projectName, stackName).Inc()

								logger.Infoln("stack [", stackName, "] bs count + 1")

							}
						}
					}
				}
			case "error":
				_, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					continue
				}

				extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
				extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
				extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

				logger.Infoln("service [", serviceMsg.name, "] bs error + 1")
				activatingServicesLoop.Delete(loopKey)
			case "active":
				countPtr, exist := activatingServicesLoop.Load(loopKey)
				if !exist {
					continue
				}

				count := atomic.LoadInt64(countPtr.(*int64))

				// for 1 instance of 1 service of 1 stack
				if count == updatingActive {
					if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" {
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName).Inc()

						logger.Infoln("stack [", stackName, "] bs success + 1")
					} else if serviceMsg.healthState == "unhealthy" {
						extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName).Inc()

						logger.Infoln("stack [", stackName, "] bs error + 1")
					}
				}

				// for 1 service of 1 stack
				if count == restarting {
					if len(serviceMsg.parentId) != 0 {
						hc := newHttpClient(10 * time.Second)
						if servicesRespBytes, err := hc.get(cattleURL + "/stacks/" + serviceMsg.parentId + "/services"); err == nil {
							_, _, _, err := jsonparser.Get(servicesRespBytes, "data", "[1]")
							if err == jsonparser.KeyPathNotFoundError {

								extendingTotalStackBootstraps.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstraps.WithLabelValues(projectName, stackName).Inc()

								logger.Infoln("stack [", stackName, "] bs count + 1")

								if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" {
									extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
									extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName).Inc()

									logger.Infoln("stack [", stackName, "] bs success + 1")
								} else if serviceMsg.healthState == "unhealthy" {
									extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
									extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName).Inc()

									logger.Infoln("stack [", stackName, "] bs error + 1")
								}
							}
						}
					}
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), upgraded, active) {
					continue
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), restarting, active) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), activating, active) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), rollingBack, active) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), updatingActive, active) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), stopped, active) {
					if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" { // healthy start
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
						extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

						logger.Infoln("service [", serviceMsg.name, "] bs success + 1")
					} else if serviceMsg.healthState == "unhealthy" { // unhealthy start
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
						extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

						logger.Infoln("service [", serviceMsg.name, "] bs error + 1")
					}

				}
			case "inactive", "removed":
				activatingServicesLoop.Delete(loopKey)
			}
		}
	}()

	// instance event handler
	go func() {
		activatingInstancesLoop := &sync.Map{}

		const (
			stopped  int64 = iota
			stopping
			starting
			running
		)

		for instanceMsg := range r.instancesBuff {
			logger.Debugf("[[instance]]: %+v", instanceMsg)

			switch instanceMsg.state {
			case "starting":
				countPtr, exist := activatingInstancesLoop.Load(instanceMsg.name)
				if !exist {
					count := stopping
					countPtr = &count
					activatingInstancesLoop.Store(instanceMsg.name, countPtr)
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), stopping, starting) { // from stopping (Instance Restart)
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name)

					logger.Infoln("instance [", instanceMsg.name, "] bs count + 1")
				}

			case "running":
				countPtr, exist := activatingInstancesLoop.Load(instanceMsg.name)
				if !exist {
					continue
				}

				if atomic.LoadInt64(countPtr.(*int64)) == stopped { // from stopped (Service Restart)
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
					extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag)
					extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag)
					extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name)

					logger.Infoln("instance [", instanceMsg.name, "] bs count + 1")
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), stopped, running) ||
					atomic.CompareAndSwapInt64(countPtr.(*int64), starting, running) {

					if len(instanceMsg.healthState) == 0 { // without health check
						go func(instanceMsg buffMsg) {
							time.Sleep(8 * time.Second)

							if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); ok {
								if atomic.LoadInt64(countPtr.(*int64)) == running {
									extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
									extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
									extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
									extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

									logger.Infoln("instance running [", instanceMsg.name, "] bs success + 1")
									activatingInstancesLoop.Delete(instanceMsg.name)
								}
							}
						}(instanceMsg)
					} else {
						if instanceMsg.healthState == "healthy" {
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

							logger.Infoln("instance [", instanceMsg.name, "] bs success + 1")
							activatingInstancesLoop.Delete(instanceMsg.name)
						} else if instanceMsg.healthState == "unhealthy" {
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

							logger.Infoln("instance [", instanceMsg.name, "] bs error + 1")
							activatingInstancesLoop.Delete(instanceMsg.name)
						}
					}
				}
			case "stopping":
				countPtr, exist := activatingInstancesLoop.Load(instanceMsg.name)
				if !exist {
					count := stopping
					activatingInstancesLoop.Store(instanceMsg.name, &count)
					continue
				}

				atomic.CompareAndSwapInt64(countPtr.(*int64), running, stopping)
			case "stopped":
				countPtr, exist := activatingInstancesLoop.Load(instanceMsg.name)
				if !exist {
					continue
				}

				if atomic.CompareAndSwapInt64(countPtr.(*int64), stopping, stopped) {
					go func(instanceMsg buffMsg) {
						if len(instanceMsg.parentId) != 0 {
							time.Sleep(16 * time.Second)

							if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); ok {
								if atomic.LoadInt64(countPtr.(*int64)) == stopped {
									// if service is active then
									hc := newHttpClient(10 * time.Second)
									if serviceRespBytes, err := hc.get(cattleURL + "/services/" + instanceMsg.parentId); err == nil {
										state, _ := jsonparser.GetString(serviceRespBytes, "state")
										if state == "active" || state == "upgraded" {
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

											logger.Infoln("instance stopped [", instanceMsg.name, "] bs success + 1")
											activatingInstancesLoop.Delete(instanceMsg.name)
										}
									}
								}
							}
						}
					}(instanceMsg)
				}
			case "removed":
				activatingInstancesLoop.Delete(instanceMsg.name)
			}
		}

	}()
}

func newRancherExporter() *rancherExporter {
	hc := newHttpClient(10 * time.Second)

	// get project self link
	projectsResponseBytes, err := hc.get(cattleURL + "/projects")
	if err != nil {
		panic(errors.New(fmt.Sprintf("cannot get project info, %v", err)))
	}

	projectBytes, _, _, err := jsonparser.Get(projectsResponseBytes, "data", "[0]")
	if err != nil {
		panic(errors.New(fmt.Sprintf("cannot get project, %v", err)))
	}

	projectId, err := jsonparser.GetString(projectBytes, "id")
	if err != nil {
		panic(errors.New(fmt.Sprintf("cannot get project id, %v", err)))
	}

	projectName, err := jsonparser.GetString(projectBytes, "name")
	if err != nil {
		panic(errors.New(fmt.Sprintf("cannot get project name, %v", err)))
	}

	projectLinksSelf, err := jsonparser.GetString(projectBytes, "links", "self")
	if err != nil {
		panic(errors.New(fmt.Sprintf("cannot get project self address, %v", err)))
	}
	if strings.HasPrefix(projectLinksSelf, "http://") {
		projectLinksSelf = strings.Replace(projectLinksSelf, "http://", "ws://", -1)
	} else {
		projectLinksSelf = strings.Replace(projectLinksSelf, "https://", "wss://", -1)
	}

	wbsFactory := func() *websocket.Conn {
		dialAddress := projectLinksSelf + "/subscribe?eventNames=resource.change&limit=-1&sockId=1"
		httpHeaders := http.Header{}
		httpHeaders.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cattleAccessKey+":"+cattleSecretKey)))
		wbs, _, err := websocket.DefaultDialer.Dial(dialAddress, httpHeaders)
		if err != nil {
			panic(err)
		}

		return wbs
	}

	result := &rancherExporter{
		projectId:     projectId,
		projectName:   projectName,
		mutex:         &sync.Mutex{},
		websocketConn: wbsFactory(),

		stacksBuff:    make(chan buffMsg, 1024),
		servicesBuff:  make(chan buffMsg, 1024),
		instancesBuff: make(chan buffMsg, 1024),

		recreateWebsocket: wbsFactory,
	}

	result.collectingExtending()

	return result
}
