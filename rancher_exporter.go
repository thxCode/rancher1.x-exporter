package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/urfave/cli"
)

const (
	// Used to prepand Prometheus metrics created by this exporter.
	namespace = "rancher"
)

var (
	log = logrus.New()

	projectLabelNames   = []string{"id", "name", "type"}
	stackLabelNames     = []string{"environment_id", "environment_name", "id", "name", "system", "type"}
	serviceLabelNames   = []string{"environment_id", "environment_name", "stack_id", "stack_name", "id", "name", "system", "type"}
	containerLabelNames = []string{"environment_id", "environment_name", "stack_id", "stack_name", "service_id", "service_name", "id", "name", "system", "type"}

	listenAddress   string
	metricPath      string
	cattleURL       string
	cattleAccessKey string
	cattleSecretKey string
	hideSys         bool
)

func main() {
	app := cli.NewApp()
	app.Name = "rancher_exporter"
	app.Version = version.Print("rancher_exporter")
	app.Usage = "A simple server that scrapes Rancher 1.6 stats and exports them via HTTP for Prometheus consumption."
	app.Action = appAction

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:   "listen_address",
			Usage:  "The address of scraping the metrics",
			EnvVar: "LISTEN_ADDRESS",
			Value:  "0.0.0.0:9173",
		},
		cli.StringFlag{
			Name:   "metric_path",
			Usage:  "The path of exposing metrics",
			EnvVar: "METRIC_PATH",
			Value:  "/metrics",
		},
		cli.StringFlag{
			Name:   "cattle_url",
			Usage:  "The URL of Rancher Server API, e.g. http://127.0.0.1:8080",
			EnvVar: "CATTLE_URL",
		},
		cli.StringFlag{
			Name:   "cattle_access_key",
			Usage:  "The access key for Rancher API",
			EnvVar: "CATTLE_ACCESS_KEY",
		},
		cli.StringFlag{
			Name:   "cattle_secret_key",
			Usage:  "The secret key for Rancher API",
			EnvVar: "CATTLE_SECRET_KEY",
		},
		cli.StringFlag{
			Name:   "log_level",
			Usage:  "Set the logging level",
			EnvVar: "LOG_LEVEL",
			Value:  "info",
		},
		cli.BoolFlag{
			Name:   "hide_sys",
			Usage:  "Hide the system metrics",
			EnvVar: "HIDE_SYS",
		},
	}

	app.Run(os.Args)
}

func appAction(c *cli.Context) error {
	ctx, cancelFn := context.WithCancel(context.Background())
	stopChan := make(chan interface{})

	// deal params
	listenAddress = c.String("listen_address")
	metricPath = c.String("metric_path")
	cattleURL = c.String("cattle_url")
	cattleAccessKey = c.String("cattle_access_key")
	cattleSecretKey = c.String("cattle_secret_key")
	hideSys = c.Bool("hide_sys")

	// set logger
	switch c.String("log_level") {
	case "debug":
		log.Level = logrus.DebugLevel
	case "warn":
		log.Level = logrus.WarnLevel
	case "fatal":
		log.Level = logrus.FatalLevel
	case "panic":
		log.Level = logrus.PanicLevel
	default:
		log.Level = logrus.InfoLevel
	}

	// cattle url
	if cattleURL == "" {
		return errors.New("cattle_url must be set and non-empty")
	} else {
		cattleURL = strings.Replace(cattleURL, "/v1", "/v2-beta", -1)

		if !strings.Contains(cattleURL, "/v2-beta") {
			cattleURL += "/v2-beta"
		}
	}

	log.Infoln("Starting rancher_exporter", version.Info(), ", with cattle URL: ", cattleURL, ", access key: ", cattleAccessKey, ", system services hidden: ", hideSys)
	log.Infoln("Build context", version.BuildContext())

	// create exporter
	rancherExporter, err := NewExporter(ctx, stopChan)
	if err != nil {
		return err
	}

	// register exporter
	prometheus.MustRegister(rancherExporter)
	prometheus.MustRegister(version.NewCollector("rancher_exporter"))

	// start web
	log.Infoln("Listening on", listenAddress)
	http.Handle(metricPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
             <head><title>Rancher 1.6 Exporter</title></head>
             <body>
             <h1>Rancher 1.6 Exporter</h1>
             <p><a href='` + metricPath + `'>Metrics</a></p>
             </body>
             </html>`))
	})
	log.Fatal(http.ListenAndServe(listenAddress, nil))

	close(stopChan)
	cancelFn()

	return nil
}

type RancherExporter struct {
	infinityWorksMetrics *InfinityWorksMetrics
	extendMetrics        *ExtendMetrics
}

func (e *RancherExporter) Describe(ch chan<- *prometheus.Desc) {
	e.extendMetrics.Describe(ch)
	e.infinityWorksMetrics.Describe(ch)
}

func (e *RancherExporter) Collect(ch chan<- prometheus.Metric) {
	e.extendMetrics.Collect(ch)
	e.infinityWorksMetrics.Collect(ch)
}

func NewExporter(ctx context.Context, stopChan <-chan interface{}) (*RancherExporter, error) {
	infinityWorksMetrics := NewInfinityWorksMetrics()

	extendMetrics := NewExtendMetrics()
	extendMetrics.Fetch(ctx, time.Second*10, stopChan)

	return &RancherExporter{
		infinityWorksMetrics,
		extendMetrics,
	}, nil
}
