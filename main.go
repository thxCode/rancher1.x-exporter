package main

import (
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/Sirupsen/logrus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/version"
	"github.com/urfave/cli"
)

var (
	listenAddress   string
	metricPath      string
	cattleURL       string
	cattleAccessKey string
	cattleSecretKey string
	hideSys         bool

	log = logrus.New()
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			log.Error(err)
		}
	}()

	app := cli.NewApp()
	app.Name = "rancher_exporter"
	app.Version = version.Print("rancher_exporter")
	app.Usage = "A simple server that scrapes Rancher 1.6 stats and exports them via HTTP for Prometheus consumption."
	app.Action = appAction

	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:        "listen_address",
			Usage:       "The address of scraping the metrics",
			EnvVar:      "LISTEN_ADDRESS",
			Value:       "0.0.0.0:9173",
			Destination: &listenAddress,
		},
		cli.StringFlag{
			Name:        "metric_path",
			Usage:       "The path of exposing metrics",
			EnvVar:      "METRIC_PATH",
			Value:       "/metrics",
			Destination: &metricPath,
		},
		cli.StringFlag{
			Name:        "cattle_url",
			Usage:       "The URL of Rancher Server API, e.g. http://127.0.0.1:8080",
			EnvVar:      "CATTLE_URL",
			Destination: &cattleURL,
		},
		cli.StringFlag{
			Name:        "cattle_access_key",
			Usage:       "The access key for Rancher API",
			EnvVar:      "CATTLE_ACCESS_KEY",
			Destination: &cattleAccessKey,
		},
		cli.StringFlag{
			Name:        "cattle_secret_key",
			Usage:       "The secret key for Rancher API",
			EnvVar:      "CATTLE_SECRET_KEY",
			Destination: &cattleSecretKey,
		},
		cli.StringFlag{
			Name:   "log_level",
			Usage:  "Set the logging level",
			EnvVar: "LOG_LEVEL",
			Value:  "debug",
		},
		cli.BoolFlag{
			Name:        "hide_sys",
			Usage:       "Hide the system metrics",
			EnvVar:      "HIDE_SYS",
			Destination: &hideSys,
		},
	}

	app.Run(os.Args)
}

func appAction(c *cli.Context) {
	stopChan := make(chan interface{}, 1)
	defer close(stopChan)

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
		panic(errors.New("cattle_url must be set and non-empty"))
	} else {
		cattleURL = strings.Replace(cattleURL, "/v1", "/v2-beta", -1)

		if !strings.Contains(cattleURL, "/v2-beta") {
			cattleURL += "/v2-beta"
		}
	}

	log.Infoln("Starting rancher_exporter", version.Info(), ", with cattle URL: ", cattleURL, ", access key: ", cattleAccessKey, ", system services hidden: ", hideSys)
	log.Infoln("Build context", version.BuildContext())

	re := newRancherExporter()

	// register exporter
	prometheus.MustRegister(re)
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

	re.Stop()
}
