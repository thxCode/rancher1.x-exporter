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

func doFetch(w io.Writer, url string) (int64, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	req.SetBasicAuth(cattleAccessKey, cattleSecretKey)
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return io.Copy(w, resp.Body)
}

type extendGeneralResponse struct {
	Data []*extendGeneralType `json:"data"`
}

type extendGeneralType struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	State          string `json:"state"`
	CreatedTS      int64  `json:"createdTS"`
	FirstRunningTS int64  `json:"firstRunningTS"`
	HealthState    string `json:"healthState"`
	System         bool   `json:"system"`
	Type           string `json:"type"`
}

var (
	// total counter of project, stack, service, instance
	totalProjectBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "project_bootstrap_total",
		Help:      "Current total number of the started projects in Rancher",
	}, projectLabelNames)

	totalProjectFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "project_failure_total",
		Help:      "Current total number of the failure projects in Rancher",
	}, projectLabelNames)

	totalStackBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stack_bootstrap_total",
		Help:      "Current total number of the started stacks in Rancher",
	}, stackLabelNames)

	totalStackFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "stack_failure_total",
		Help:      "Current total number of the failure stacks in Rancher",
	}, stackLabelNames)

	totalServiceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "service_bootstrap_total",
		Help:      "Current total number of the started services in Rancher",
	}, serviceLabelNames)

	totalServiceFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "service_failure_total",
		Help:      "Current total number of the failure services in Rancher",
	}, serviceLabelNames)

	totalInstanceBootstrap = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instance_bootstrap_total",
		Help:      "Current total number of the started containers in Rancher",
	}, containerLabelNames)

	totalInstanceFailure = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: namespace,
		Name:      "instance_failure_total",
		Help:      "Current total number of the failure containers in Rancher",
	}, containerLabelNames)

	// startup gauge
	instanceBootstrapMsCost = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Namespace: namespace,
		Name:      "instance_startup_ms",
		Help:      "The startup milliseconds of container in Rancher",
	}, containerLabelNames)
)

type ExtendMetrics struct {
	m *sync.RWMutex

	Projects map[string]*RancherProject `json:"projects"`
}

func (o *ExtendMetrics) Describe(ch chan<- *prometheus.Desc) {
	totalProjectBootstrap.Describe(ch)
	totalProjectFailure.Describe(ch)
	totalStackBootstrap.Describe(ch)
	totalStackFailure.Describe(ch)
	totalServiceBootstrap.Describe(ch)
	totalServiceFailure.Describe(ch)
	totalInstanceBootstrap.Describe(ch)
	totalInstanceFailure.Describe(ch)
	instanceBootstrapMsCost.Describe(ch)
}

func (o *ExtendMetrics) Collect(ch chan<- prometheus.Metric) {
	o.m.RLock()

	// counter metrics
	totalProjectBootstrap.Collect(ch)
	totalProjectFailure.Collect(ch)
	totalStackBootstrap.Collect(ch)
	totalStackFailure.Collect(ch)
	totalServiceBootstrap.Collect(ch)
	totalServiceFailure.Collect(ch)
	totalInstanceBootstrap.Collect(ch)
	totalInstanceFailure.Collect(ch)
	// gauge metrics
	instanceBootstrapMsCost.Collect(ch)

	o.m.RUnlock()
}

func (o *ExtendMetrics) Fetch(ctx context.Context, interval time.Duration, stopChan <-chan interface{}) {
	objectFetch := func() {
		o.m.Lock()

		var projectsBuffer bytes.Buffer
		if _, err := doFetch(&projectsBuffer, cattleURL+"/projects"); err != nil {
			log.Error(err)
		} else {
			var tmp extendGeneralResponse

			if err := json.Unmarshal(projectsBuffer.Bytes(), &tmp); err != nil {
				log.Fatal(err)
			} else {
				for _, d := range tmp.Data {

					if take, ok := o.Projects[d.Name]; !ok {
						o.Projects[d.Name] = &RancherProject{
							ID:          d.ID,
							Name:        d.Name,
							State:       d.State,
							CreatedTS:   d.CreatedTS,
							HealthState: d.HealthState,
							Type:        d.Type,

							Stacks: make(map[string]*RancherStack),

							BootstrapCount: 1,
							FailureCount:   0,
						}
						totalProjectBootstrap.WithLabelValues(d.ID, d.Name, d.Type).Inc()
						totalProjectFailure.WithLabelValues(d.ID, d.Name, d.Type)
					} else {
						if (take.State != d.State && d.State == "active") || (take.HealthState != d.HealthState && d.HealthState == "healthy") {
							totalProjectBootstrap.WithLabelValues(d.ID, d.Name, d.Type).Inc()
							take.BootstrapCount += 1
						}
						if take.HealthState != d.HealthState && d.HealthState == "unhealthy" {
							totalProjectFailure.WithLabelValues(d.ID, d.Name, d.Type).Inc()
							take.FailureCount += 1
						}
						take.State = d.State
						take.ID = d.ID
						take.Type = d.Type
						take.CreatedTS = d.CreatedTS
						take.HealthState = d.HealthState
					}
				}

				ctx, cancelFn := context.WithTimeout(ctx, interval*2)
				defer cancelFn()

				var wg = &sync.WaitGroup{}
				for _, p := range o.Projects {
					wg.Add(1)
					go func(p *RancherProject) {
						defer wg.Done()

						p.Fetch(ctx)
					}(p)
				}
				wg.Wait()
			}
		}

		o.m.Unlock()
	}

	objectFetch()

	go func() {
		tickChan := time.NewTicker(interval).C

		for {
			select {
			case <-tickChan:
				objectFetch()
			case <-stopChan:
				break
			}
		}
	}()
}

type RancherProject struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	CreatedTS   int64  `json:"createdTS"`
	HealthState string `json:"healthState"`
	Type        string `json:"type"`

	Stacks map[string]*RancherStack `json:"stack"`

	BootstrapCount uint64 `json:"bootstrapCount"`
	FailureCount   uint64 `json:"failureCount"`
}

func (p *RancherProject) Fetch(ctx context.Context) {
	var stacksBuffer bytes.Buffer
	if _, err := doFetch(&stacksBuffer, cattleURL+"/projects/"+p.ID+"/stacks?limit=100&sort=id"); err != nil {
		log.Error(err)
	} else {
		var tmp extendGeneralResponse

		if err := json.Unmarshal(stacksBuffer.Bytes(), &tmp); err != nil {
			log.Fatal(err)
		} else {
			for _, d := range tmp.Data {
				if hideSys && d.System {
					continue
				}

				systemStr := strconv.FormatBool(d.System)

				if take, ok := p.Stacks[d.Name]; !ok {
					p.Stacks[d.Name] = &RancherStack{
						ID:          d.ID,
						Name:        d.Name,
						State:       d.State,
						CreatedTS:   d.CreatedTS,
						HealthState: d.HealthState,
						System:      d.System,
						Type:        d.Type,

						Services: make(map[string]*RancherService),

						BootstrapCount: 1,
						FailureCount:   0,

						parent: p,
					}
					totalStackBootstrap.WithLabelValues(p.ID, p.Name, d.ID, d.Name, systemStr, d.Type).Inc()
					totalStackFailure.WithLabelValues(p.ID, p.Name, d.ID, d.Name, systemStr, d.Type)
				} else {
					if (take.State != d.State && d.State == "active") || (take.HealthState != d.HealthState && d.HealthState == "healthy") {
						totalStackBootstrap.WithLabelValues(p.ID, p.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.BootstrapCount += 1
					}
					if take.HealthState != d.HealthState && d.HealthState == "unhealthy" {
						totalStackFailure.WithLabelValues(p.ID, p.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.FailureCount += 1
					}
					take.State = d.State
					take.ID = d.ID
					take.CreatedTS = d.CreatedTS
					take.HealthState = d.HealthState
					take.System = d.System
					take.Type = d.Type
				}
			}

			var wg = &sync.WaitGroup{}
			for _, s := range p.Stacks {
				wg.Add(1)
				go func(s *RancherStack) {
					defer wg.Done()

					s.Fetch(ctx)
				}(s)
			}
			wg.Wait()
		}
	}
}

type RancherStack struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	CreatedTS   int64  `json:"createdTS"`
	HealthState string `json:"healthState"`
	System      bool   `json:"system"`
	Type        string `json:"type"`

	Services map[string]*RancherService `json:"services"`

	BootstrapCount uint64 `json:"bootstrapCount"`
	FailureCount   uint64 `json:"failureCount"`

	parent *RancherProject
}

func (s *RancherStack) Fetch(ctx context.Context) {
	var servicesBuffer bytes.Buffer
	if _, err := doFetch(&servicesBuffer, cattleURL+"/stacks/"+s.ID+"/services?limit=100&sort=id"); err != nil {
		log.Error(err)
	} else {
		var tmp extendGeneralResponse

		if err := json.Unmarshal(servicesBuffer.Bytes(), &tmp); err != nil {
			log.Fatal(err)
		} else {
			for _, d := range tmp.Data {
				if hideSys && d.System {
					continue
				}

				systemStr := strconv.FormatBool(d.System)

				if take, ok := s.Services[d.Name]; !ok {
					s.Services[d.Name] = &RancherService{
						ID:          d.ID,
						Name:        d.Name,
						State:       d.State,
						CreatedTS:   d.CreatedTS,
						HealthState: d.HealthState,
						System:      d.System,
						Type:        d.Type,

						Instances: make(map[string]*RancherInstance),

						BootstrapCount: 1,
						FailureCount:   0,

						parent: s,
					}
					totalServiceBootstrap.WithLabelValues(s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
					totalServiceFailure.WithLabelValues(s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type)
				} else {
					if (take.State != d.State && d.State == "active") || (take.HealthState != d.HealthState && d.HealthState == "healthy") {
						totalServiceBootstrap.WithLabelValues(s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.BootstrapCount += 1
					}
					if take.HealthState != d.HealthState && d.HealthState == "unhealthy" {
						totalServiceFailure.WithLabelValues(s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.FailureCount += 1
					}
					take.State = d.State
					take.ID = d.ID
					take.CreatedTS = d.CreatedTS
					take.HealthState = d.HealthState
					take.System = d.System
				}
			}

			var wg = &sync.WaitGroup{}
			for _, s := range s.Services {
				wg.Add(1)
				go func(s *RancherService) {
					defer wg.Done()

					s.Fetch(ctx)
				}(s)
			}
			wg.Wait()
		}
	}
}

type RancherService struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	State       string `json:"state"`
	CreatedTS   int64  `json:"createdTS"`
	HealthState string `json:"healthState"`
	System      bool   `json:"system"`
	Type        string `json:"type"`

	Instances map[string]*RancherInstance `json:"instances"`

	BootstrapCount uint64 `json:"bootstrapCount"`
	FailureCount   uint64 `json:"failureCount"`

	parent *RancherStack
}

func (s *RancherService) Fetch(ctx context.Context) {
	var instancesBuffer bytes.Buffer
	if _, err := doFetch(&instancesBuffer, cattleURL+"/services/"+s.ID+"/instances?limit=100&sort=id"); err != nil {
		log.Error(err)
	} else {
		var tmp extendGeneralResponse

		if err := json.Unmarshal(instancesBuffer.Bytes(), &tmp); err != nil {
			log.Fatal(err)
		} else {
			for _, d := range tmp.Data {
				if hideSys && d.System {
					continue
				}

				systemStr := strconv.FormatBool(d.System)
				startUpTS := d.FirstRunningTS - d.CreatedTS

				if take, ok := s.Instances[d.Name]; !ok {
					s.Instances[d.Name] = &RancherInstance{
						ID:             d.ID,
						Name:           d.Name,
						State:          d.State,
						CreatedTS:      d.CreatedTS,
						FirstRunningTS: d.FirstRunningTS,
						HealthState:    d.HealthState,
						System:         d.System,
						Type:           d.Type,

						StartUpTS: startUpTS,

						BootstrapCount: 1,
						FailureCount:   0,

						parent: s,
					}

					totalInstanceBootstrap.WithLabelValues(s.parent.parent.ID, s.parent.parent.Name, s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
					totalInstanceFailure.WithLabelValues(s.parent.parent.ID, s.parent.parent.Name, s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type)
				} else {
					if (take.State != d.State && d.State == "running") || (take.HealthState != d.HealthState && d.HealthState == "healthy") {
						totalInstanceBootstrap.WithLabelValues(s.parent.parent.ID, s.parent.parent.Name, s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.BootstrapCount += 1
					}
					if take.HealthState != d.HealthState && d.HealthState == "unhealthy" {
						totalInstanceFailure.WithLabelValues(s.parent.parent.ID, s.parent.parent.Name, s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Inc()
						take.FailureCount += 1
					}
					take.State = d.State
					take.ID = d.ID
					take.CreatedTS = d.CreatedTS
					take.HealthState = d.HealthState
					take.System = d.System
					take.Type = d.Type
					take.StartUpTS = startUpTS
				}

				instanceBootstrapMsCost.WithLabelValues(s.parent.parent.ID, s.parent.parent.Name, s.parent.ID, s.parent.Name, s.ID, s.Name, d.ID, d.Name, systemStr, d.Type).Set(float64(startUpTS))
			}
		}
	}
}

type RancherInstance struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	State          string `json:"state"`
	CreatedTS      int64  `json:"createdTS"`
	FirstRunningTS int64  `json:"firstRunningTS"`
	HealthState    string `json:"healthState"`
	System         bool   `json:"system"`
	Type           string `json:"type"`

	StartUpTS int64 `json:"startUpTS"`

	BootstrapCount uint64 `json:"bootstrapCount"`
	FailureCount   uint64 `json:"failureCount"`

	parent *RancherService
}

func NewExtendMetrics() *ExtendMetrics {
	return &ExtendMetrics{
		m:        &sync.RWMutex{},
		Projects: make(map[string]*RancherProject),
	}
}
