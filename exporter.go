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

	"github.com/buger/jsonparser"
	"github.com/gorilla/websocket"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/thxcode/rancher1.x-restarting-controller/pkg/utils"
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
	extendingTotalStackBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
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
	projectName string
	projectId   string

	mutex        *sync.Mutex
	wbs          *websocket.Conn
	wbsReconnect func() *websocket.Conn

	stacksBuff    chan buffMsg
	servicesBuff  chan buffMsg
	instancesBuff chan buffMsg
}

func (r *rancherExporter) Describe(ch chan<- *prometheus.Desc) {
	infinityWorksStacksHealth.Describe(ch)
	infinityWorksStacksState.Describe(ch)
	infinityWorksServicesScale.Describe(ch)
	infinityWorksServicesHealth.Describe(ch)
	infinityWorksServicesState.Describe(ch)
	infinityWorksHostsState.Describe(ch)
	infinityWorksHostAgentsState.Describe(ch)

	extendingTotalStackBootstrap.Describe(ch)
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
	if r.wbs != nil {
		r.wbs.Close()
	}

	close(r.instancesBuff)
	close(r.servicesBuff)
	close(r.stacksBuff)
}

func (r *rancherExporter) asyncMetrics(ch chan<- prometheus.Metric) {
	// collect
	extendingTotalStackBootstrap.Collect(ch)
	extendingTotalSuccessStackBootstrap.Collect(ch)
	extendingTotalErrorStackBootstrap.Collect(ch)

	extendingTotalServiceBootstraps.Collect(ch)
	extendingTotalSuccessServiceBootstrap.Collect(ch)
	extendingTotalErrorServiceBootstrap.Collect(ch)

	extendingTotalInstanceBootstraps.Collect(ch)
	extendingTotalSuccessInstanceBootstrap.Collect(ch)
	extendingTotalErrorInstanceBootstrap.Collect(ch)
	extendingInstanceBootstrapMsCost.Collect(ch)
}

func (r *rancherExporter) syncMetrics(ch chan<- prometheus.Metric) {
	projectName := r.projectName
	projectId := r.projectId

	defer func() {
		if err := recover(); err != nil {
			log.Errorln(err)
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
			log.Warnln(err)
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
				log.Errorln(stacksAddress, err)
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
								log.Errorln(servicesAddress, err)
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
												log.Errorln(instancesAddress, err)
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
	glog := utils.GetGlobalLogger()

	projectName := r.projectName
	projectId := r.projectId

	stackIdNameMap := &sync.Map{}

	go func() {
		hc := newHttpClient(30 * time.Second)
		stacksAddress := cattleURL + "/projects/" + projectId + "/stacks?limit=100&sort=id"
		if hideSys {
			stacksAddress += "&system=false"
		}

		stkwg := &sync.WaitGroup{}
		for {
			if stacksRespBytes, err := hc.get(stacksAddress); err != nil {
				log.Errorln(stacksAddress, err)
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

						switch stackState {
						case "active":
							if stackHealthState == "unhealthy" {
								extendingTotalStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstrap.WithLabelValues(projectName, stackName).Inc()

								extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName).Inc()
							} else if stackHealthState == "healthy" {
								extendingTotalStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalStackBootstrap.WithLabelValues(projectName, stackName).Inc()

								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackName).Inc()
							}
						case "error":
							extendingTotalStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalStackBootstrap.WithLabelValues(projectName, stackName).Inc()

							extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackName).Inc()
						}

						servicesAddress := cattleURL + "/stacks/" + stackId + "/services?limit=100&sort=id"
						if hideSys {
							servicesAddress += "&system=false"
						}

						svcwg := &sync.WaitGroup{}
						for {
							if servicesRespBytes, err := hc.get(servicesAddress); err != nil {
								log.Errorln(servicesAddress, err)
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

										switch serviceState {
										case "active":
											extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
											extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
											extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceName).Inc()

											if serviceHealthState == "unhealthy" {
												extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
												extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
												extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceName).Inc()
											} else if serviceHealthState == "healthy" {
												extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
												extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
												extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceName).Inc()
											}
										case "error":
											extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
											extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
											extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceName).Inc()

											extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
											extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
											extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceName).Inc()
										}

										instancesAddress := cattleURL + "/services/" + serviceId + "/instances?limit=100&sort=id"
										if hideSys {
											instancesAddress += "&system=false"
										}

										for {
											if instancesRespBytes, err := hc.get(instancesAddress); err != nil {
												log.Errorln(instancesAddress, err)
												break
											} else {
												jsonparser.ArrayEach(instancesRespBytes, func(instanceBytes []byte, dataType jsonparser.ValueType, offset int, err error) {

													instanceName, _ := jsonparser.GetString(instanceBytes, "name")
													instanceSystem, _ := jsonparser.GetUnsafeString(instanceBytes, "system")
													instanceType, _ := jsonparser.GetString(instanceBytes, "type")
													instanceState, _ := jsonparser.GetString(instanceBytes, "state")
													instanceFirstRunningTS, _ := jsonparser.GetInt(instanceBytes, "firstRunningTS")
													instanceCreatedTS, _ := jsonparser.GetInt(instanceBytes, "createdTS")

													switch instanceState {
													case "stopped":
														fallthrough
													case "running":
														extendingTotalInstanceBootstraps.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
														extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, specialTag, specialTag).Inc()
														extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, serviceName, specialTag).Inc()
														extendingTotalInstanceBootstraps.WithLabelValues(projectName, stackName, serviceName, instanceName).Inc()

														if instanceFirstRunningTS != 0 {
															instanceStartupTime := instanceFirstRunningTS - instanceCreatedTS
															extendingInstanceBootstrapMsCost.WithLabelValues(projectName, stackName, serviceName, instanceName, instanceSystem, instanceType).Set(float64(instanceStartupTime))
														}

														extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
														extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, specialTag, specialTag).Inc()
														extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, specialTag).Inc()
														extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, stackName, serviceName, instanceName).Inc()
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

		for {
		recall:
			_, messageBytes, err := r.wbs.ReadMessage()
			if err != nil {
				glog.Warnln("reconnect websocket")
				r.wbs = r.wbsReconnect()
				goto recall
			}

			if resourceType, _ := jsonparser.GetString(messageBytes, "resourceType"); len(resourceType) != 0 {
				resourceBytes, _, _, err := jsonparser.Get(messageBytes, "data", "resource")
				if err != nil {
					glog.Warnln(err)
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
						name:          name,
						state:         state,
						healthState:   healthState,
						transitioning: transitioning,
						stackName:     stackName,
					}
				case "instance":
					name, _ := jsonparser.GetString(resourceBytes, "name")
					state, _ := jsonparser.GetString(resourceBytes, "state")
					healthState, _ := jsonparser.GetString(resourceBytes, "healthState")
					transitioning, _ := jsonparser.GetString(resourceBytes, "transitioning")
					labelStackServiceName, _ := jsonparser.GetString(resourceBytes, "labels", "io.rancher.stack_service.name")

					labelStackServiceNameSplit := strings.Split(labelStackServiceName, "/")

					r.instancesBuff <- buffMsg{
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

	go func() {
		activatingStackLoop := make(map[string]int32, 16)

		for stackMsg := range r.stacksBuff {
			if stackMsg.state == "removed" {
				stackIdNameMap.Delete(stackMsg.id)
				delete(activatingStackLoop, stackMsg.name)
			} else if stackMsg.transitioning == "no" {
				if looping, ok := activatingStackLoop[stackMsg.name]; ok {
					if looping == 0 {
						if stackMsg.state == "active" {
							if stackMsg.healthState == "healthy" {
								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

								glog.Infoln("stack [", stackMsg.name, "] bs success + 1")
								activatingStackLoop[stackMsg.name] = 1
							} else if stackMsg.healthState == "unhealthy" {
								extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
								extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

								glog.Infoln("stack [", stackMsg.name, "] bs error + 1")
								activatingStackLoop[stackMsg.name] = 1
							}
						} else if stackMsg.state == "error" {
							extendingTotalErrorStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
							extendingTotalErrorStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

							glog.Infoln("stack [", stackMsg.name, "] bs error + 1")
							activatingStackLoop[stackMsg.name] = 1
						}
					} else {
						if stackMsg.state == "active" && stackMsg.healthState == "unhealthy" { // inactive
							delete(activatingStackLoop, stackMsg.name)
						}
					}
				} else if stackMsg.state == "active" && stackMsg.healthState == "healthy" { // empty stack start
					if _, ok := stackIdNameMap.Load(stackMsg.id); !ok {
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalSuccessStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()
						extendingTotalStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
						extendingTotalStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

						glog.Infoln("stack [", stackMsg.name, "] bs count + 1")
						glog.Infoln("stack [", stackMsg.name, "] bs success + 1")
					}
					activatingStackLoop[stackMsg.name] = 1
				}
			} else if _, ok := activatingStackLoop[stackMsg.name]; !ok && stackMsg.state == "activating" && stackMsg.healthState == "unhealthy" { // starting
				extendingTotalStackBootstrap.WithLabelValues(projectName, specialTag).Inc()
				extendingTotalStackBootstrap.WithLabelValues(projectName, stackMsg.name).Inc()

				glog.Infoln("stack [", stackMsg.name, "] bs count + 1")
				activatingStackLoop[stackMsg.name] = 0
			}
		}
	}()

	go func() {
		activatingServicesLoop := make(map[string]int32, 32)

		for serviceMsg := range r.servicesBuff {
			stackName := serviceMsg.stackName
			loopKey := stackName + "-" + serviceMsg.name

			if serviceMsg.state == "removed" {
				delete(activatingServicesLoop, loopKey)
			} else if serviceMsg.transitioning == "no" {
				if looping, ok := activatingServicesLoop[loopKey]; ok {
					if looping <= 0 { // [active]
						if serviceMsg.state == "active" {
							if serviceMsg.healthState == "healthy" || serviceMsg.healthState == "started-once" { // healthy start
								extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
								extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
								extendingTotalSuccessServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

								glog.Infoln("service [", serviceMsg.name, "] bs success + 1")
								activatingServicesLoop[loopKey] = 1
							} else if serviceMsg.healthState == "unhealthy" { // unhealthy start
								extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
								extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
								extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

								glog.Infoln("service [", serviceMsg.name, "] bs error + 1")
								activatingServicesLoop[loopKey] = 1
							}
						} else if serviceMsg.state == "error" { // error start
							extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
							extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
							extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

							glog.Infoln("service [", serviceMsg.name, "] bs error + 1")
							activatingServicesLoop[loopKey] = 1
						}
					} else if looping == 1 && serviceMsg.state == "inactive" {
						delete(activatingServicesLoop, loopKey)
					}
				}
			} else if looping, ok := activatingServicesLoop[loopKey]; !ok {
				if serviceMsg.state == "activating" && serviceMsg.healthState == "healthy" { // [starting] -> count bs 1
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					glog.Infoln("service [", serviceMsg.name, "] bs count + 1")
					activatingServicesLoop[loopKey] = 0
				} else if serviceMsg.state == "restarting" && serviceMsg.healthState == "healthy" {
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					glog.Infoln("service [", serviceMsg.name, "] bs count + 1")
					activatingServicesLoop[loopKey] = 0
				}
			} else {
				if looping == 0 && serviceMsg.state == "updating-active" && serviceMsg.healthState == "unhealthy" { // error start
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalErrorServiceBootstrap.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					glog.Infoln("service [", serviceMsg.name, "] bs error + 1")
					activatingServicesLoop[loopKey] = -1
				} else if looping == 1 && serviceMsg.state == "restarting" && serviceMsg.healthState == "healthy" { // [restarting] -> count bs 1
					extendingTotalServiceBootstraps.WithLabelValues(projectName, specialTag, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, specialTag).Inc()
					extendingTotalServiceBootstraps.WithLabelValues(projectName, stackName, serviceMsg.name).Inc()

					glog.Infoln("service [", serviceMsg.name, "] bs count + 1")
					activatingServicesLoop[loopKey] = 0
				}
			}
		}
	}()

	go func() {
		activatingInstancesLoop := &sync.Map{}

		runningStopChan := make(chan string, 16)
		defer close(runningStopChan)

		stoppedStopChan := make(chan string, 16)
		defer close(stoppedStopChan)

		for instanceMsg := range r.instancesBuff {
			if instanceMsg.state == "removed" {
				activatingInstancesLoop.Delete(instanceMsg.name)
			} else if instanceMsg.transitioning == "no" {
				if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); ok {
					if instanceMsg.state == "running" && atomic.CompareAndSwapInt32(countPtr.(*int32), 0, 1) {
						if len(instanceMsg.healthState) == 0 {
							go func(instanceMsg buffMsg) {
								after := time.After(8 * time.Second)

								for {
									select {
									case stopName := <-runningStopChan:
										if stopName == instanceMsg.name {
											return
										}
									case <-after:
										if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); ok {
											if atomic.LoadInt32(countPtr.(*int32)) == 1 {
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
												extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

												glog.Infoln("instance running [", instanceMsg.name, "] bs success + 1")
												activatingInstancesLoop.Delete(instanceMsg.name)
											}
										}
										return
									}
								}
							}(instanceMsg)
						} else if instanceMsg.healthState == "healthy" {
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
							extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

							glog.Infoln("instance [", instanceMsg.name, "] bs success + 1")
							activatingInstancesLoop.Delete(instanceMsg.name)
						} else if instanceMsg.healthState == "unhealthy" {
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
							extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

							glog.Infoln("instance [", instanceMsg.name, "] bs error + 1")
							activatingInstancesLoop.Delete(instanceMsg.name)
						}
					} else if instanceMsg.state == "stopped" && atomic.CompareAndSwapInt32(countPtr.(*int32), 1, 3) {
						runningStopChan <- instanceMsg.name

						go func(instanceMsg buffMsg) {
							after := time.After(16 * time.Second)

							for {
								select {
								case stopName := <-stoppedStopChan:
									if stopName == instanceMsg.name {
										return
									}
								case <-after:
									if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); ok {
										if atomic.LoadInt32(countPtr.(*int32)) == 3 {
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
											extendingTotalSuccessInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

											glog.Infoln("instance stopped [", instanceMsg.name, "] bs success + 1")
											activatingInstancesLoop.Delete(instanceMsg.name)
										}
									}
									return
								}
							}
						}(instanceMsg)
					}
				}
			} else {
				if countPtr, ok := activatingInstancesLoop.Load(instanceMsg.name); !ok {
					if instanceMsg.state == "starting" {
						extendingTotalInstanceBootstraps.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
						extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
						extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
						extendingTotalInstanceBootstraps.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

						glog.Infoln("instance [", instanceMsg.name, "] bs count + 1")
						count := int32(0)
						activatingInstancesLoop.Store(instanceMsg.name, &count)
					}
				} else if atomic.CompareAndSwapInt32(countPtr.(*int32), 3, 4) { // starting -> stopped -> starting
					if instanceMsg.state == "starting" {
						stoppedStopChan <- instanceMsg.name

						extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, specialTag, specialTag, specialTag).Inc()
						extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, specialTag, specialTag).Inc()
						extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, specialTag).Inc()
						extendingTotalErrorInstanceBootstrap.WithLabelValues(projectName, instanceMsg.stackName, instanceMsg.serviceName, instanceMsg.name).Inc()

						glog.Infoln("instance [", instanceMsg.name, "] bs error + 1")
						activatingInstancesLoop.Delete(instanceMsg.name)
					}
				}
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

	// create wbs
	dialAddress := projectLinksSelf + "/subscribe?eventNames=resource.change&limit=-1&sockId=1"
	httpHeaders := http.Header{}
	httpHeaders.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cattleAccessKey+":"+cattleSecretKey)))
	wbs, _, err := websocket.DefaultDialer.Dial(dialAddress, httpHeaders)
	if err != nil {
		panic(err)
	}

	result := &rancherExporter{
		projectName: projectName,
		projectId:   projectId,

		mutex: &sync.Mutex{},
		wbs:   wbs,
		wbsReconnect: func() *websocket.Conn {
			dialAddress := projectLinksSelf + "/subscribe?eventNames=resource.change&limit=-1&sockId=1"
			httpHeaders := http.Header{}
			httpHeaders.Add("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(cattleAccessKey+":"+cattleSecretKey)))
			wbs, _, err := websocket.DefaultDialer.Dial(dialAddress, httpHeaders)
			if err != nil {
				panic(err)
			}

			return wbs
		},

		stacksBuff:    make(chan buffMsg, 16),
		servicesBuff:  make(chan buffMsg, 16),
		instancesBuff: make(chan buffMsg, 16),
	}

	result.collectingExtending()

	return result
}
