package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	agentStates   = []string{"activating", "active", "reconnecting", "disconnected", "disconnecting", "finishing-reconnect", "reconnected"}
	hostStates    = []string{"activating", "active", "deactivating", "error", "erroring", "inactive", "provisioned", "purged", "purging", "registering", "removed", "removing", "requested", "restoring", "updating_active", "updating_inactive"}
	stackStates   = []string{"activating", "active", "canceled_upgrade", "canceling_upgrade", "error", "erroring", "finishing_upgrade", "removed", "removing", "requested", "restarting", "rolling_back", "updating_active", "upgraded", "upgrading"}
	serviceStates = []string{"activating", "active", "canceled_upgrade", "canceling_upgrade", "deactivating", "finishing_upgrade", "inactive", "registering", "removed", "removing", "requested", "restarting", "rolling_back", "updating_active", "updating_inactive", "upgraded", "upgrading"}
	healthStates  = []string{"healthy", "unhealthy"}
	endpoints     = []string{"hosts", "stacks", "services"}

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
		}, []string{"name", "stack_name"})

	servicesHealth = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_health_status",
			Help:      "HealthState of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "health_state"})

	servicesState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "service_state",
			Help:      "State of the service, as reported by the Rancher API",
		}, []string{"id", "stack_id", "name", "stack_name", "state"})
)

type InfinityWorksMetrics struct {
	m *sync.RWMutex
}

func (o *InfinityWorksMetrics) Describe(ch chan<- *prometheus.Desc) {
	stacksHealth.Describe(ch)
	stacksState.Describe(ch)
	servicesScale.Describe(ch)
	servicesHealth.Describe(ch)
	servicesState.Describe(ch)
	hostsState.Describe(ch)
	hostAgentsState.Describe(ch)
}

func (o *InfinityWorksMetrics) Collect(ch chan<- prometheus.Metric) {
	o.m.RLock()

	// reset
	stacksHealth.Reset()
	stacksState.Reset()
	servicesScale.Reset()
	servicesHealth.Reset()
	servicesState.Reset()
	hostsState.Reset()
	hostAgentsState.Reset()

	o.collect()

	stacksHealth.Collect(ch)
	stacksState.Collect(ch)
	servicesScale.Collect(ch)
	servicesHealth.Collect(ch)
	servicesState.Collect(ch)
	hostsState.Collect(ch)
	hostAgentsState.Collect(ch)

	o.m.RUnlock()
}

type infinityWorksGeneralResponse struct {
	Data []struct {
		HealthState string `json:"healthState"`
		Name        string `json:"name"`
		State       string `json:"state"`
		System      bool   `json:"system"`
		Scale       int    `json:"scale"`
		HostName    string `json:"hostname"`
		ID          string `json:"id"`
		StackID     string `json:"stackId"`
		EnvID       string `json:"environmentId"`
		Type        string `json:"type"`
		AgentState  string `json:"agentState"`
	} `json:"data"`
}

func (o *InfinityWorksMetrics) collect() {
	stackRef := make(map[string]string)

	for _, p := range endpoints {

		var data, err = gatherData(p)
		if err != nil {
			log.Error("Error getting JSON from URL ", p)
			return
		}

		if err := processMetrics(data, p, stackRef); err != nil {
			log.Errorf("Error scraping rancher url: %s", err)
			return
		}
	}
}

func processMetrics(data *infinityWorksGeneralResponse, endpoint string, stackRef map[string]string) error {
	for _, x := range data.Data {
		if hideSys && x.System {
			continue
		}

		switch endpoint {
		case "hosts":
			s := x.HostName
			if x.Name != "" {
				s = x.Name
			}

			if err := setHostMetrics(x.ID, s, x.State, x.AgentState); err != nil {
				log.Errorf("Error processing host metrics: %s", err)
				log.Errorf("Attempt Failed to set %s, %s, [agent] %s ", x.HostName, x.State, x.AgentState)

				continue
			}
		case "stacks":
			stackRef[x.ID] = x.Name

			if err := setStackMetrics(x.ID, x.Name, x.State, x.HealthState, strconv.FormatBool(x.System)); err != nil {
				log.Errorf("Error processing stack metrics: %s", err)
				log.Errorf("Attempt Failed to set %s, %s, %s, %t", x.Name, x.State, x.HealthState, x.System)

				continue
			}
		case "services":
			if stackName, ok := stackRef[x.StackID]; ok {
				if err := setServiceMetrics(x.ID, x.Name, x.StackID, stackName, x.State, x.HealthState, x.Scale); err != nil {
					log.Errorf("Error processing service metrics: %s", err)
					log.Errorf("Attempt Failed to set %s, %s, %s, %s, %d", x.Name, stackName, x.State, x.HealthState, x.Scale)

					continue
				}
			}
		}
	}

	return nil
}

func setHostMetrics(id, name string, state, agentState string) error {
	for _, y := range hostStates {
		if state == y {
			hostsState.WithLabelValues(id, name, y).Set(1)
		} else {
			hostsState.WithLabelValues(id, name, y).Set(0)
		}

	}

	for _, y := range agentStates {
		if agentState == y {
			hostAgentsState.WithLabelValues(id, name, y).Set(1)
		} else {
			hostAgentsState.WithLabelValues(id, name, y).Set(0)
		}

	}

	return nil
}

func setStackMetrics(id, name string, state string, health string, system string) error {
	for _, y := range healthStates {
		if health == y {
			stacksHealth.WithLabelValues(id, name, y, system).Set(1)
		} else {
			stacksHealth.WithLabelValues(id, name, y, system).Set(0)
		}
	}

	for _, y := range stackStates {
		if state == y {
			stacksState.WithLabelValues(id, name, y, system).Set(1)
		} else {
			stacksState.WithLabelValues(id, name, y, system).Set(0)
		}

	}

	return nil
}

func setServiceMetrics(id, name string, stackID, stack string, state string, health string, scale int) error {
	servicesScale.WithLabelValues(name, stack).Set(float64(scale))

	for _, y := range healthStates {
		if health == y {
			servicesHealth.WithLabelValues(id, stackID, name, stack, y).Set(1)
		} else {
			servicesHealth.WithLabelValues(id, stackID, name, stack, y).Set(0)
		}
	}

	for _, y := range serviceStates {
		if state == y {
			servicesState.WithLabelValues(id, stackID, name, stack, y).Set(1)
		} else {
			servicesState.WithLabelValues(id, stackID, name, stack, y).Set(0)
		}

	}

	return nil
}

func gatherData(endpoint string) (*infinityWorksGeneralResponse, error) {
	resp := new(infinityWorksGeneralResponse)

	err := getJSON(cattleURL+"/"+endpoint, cattleAccessKey, cattleSecretKey, &resp)
	if err != nil {
		log.Error("Error getting JSON from endpoint ", endpoint)
		return nil, err
	}

	return resp, err
}

func getJSON(url string, accessKey string, secretKey string, target interface{}) error {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Error("Error Collecting JSON from API: ", err)
	}

	req.SetBasicAuth(accessKey, secretKey)
	resp, err := client.Do(req)
	if err != nil {
		log.Error("Error Collecting JSON from API: ", err)
	}
	defer resp.Body.Close()

	respFormatted := json.NewDecoder(resp.Body).Decode(target)

	return respFormatted
}

func NewInfinityWorksMetrics() *InfinityWorksMetrics {
	return &InfinityWorksMetrics{
		m: &sync.RWMutex{},
	}
}
