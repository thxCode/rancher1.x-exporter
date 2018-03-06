# Rancher1.x exporter of Prometheus

An exporter exposes some [metrics](METRICS.md) of Rancher1.x to Prometheus.

[![](https://img.shields.io/badge/Github-thxcode/rancher1.x--exporter-orange.svg)](https://github.com/thxcode/rancher1.x-exporter)&nbsp;[![](https://img.shields.io/badge/Docker_Hub-maiwj/rancher1.x--exporter-orange.svg)](https://hub.docker.com/r/maiwj/rancher1.x-exporter)&nbsp;[![](https://img.shields.io/docker/build/maiwj/rancher1.x-exporter.svg)](https://hub.docker.com/r/maiwj/rancher1.x-exporter)&nbsp;[![](https://img.shields.io/docker/pulls/maiwj/rancher1.x-exporter.svg)](https://store.docker.com/community/images/maiwj/rancher1.x-exporter)&nbsp;[![](https://img.shields.io/github/license/thxcode/rancher1.x-exporter.svg)](https://github.com/thxcode/rancher1.x-exporter)

[![](https://images.microbadger.com/badges/image/maiwj/rancher1.x-exporter.svg)](https://microbadger.com/images/maiwj/rancher1.x-exporter)&nbsp;[![](https://images.microbadger.com/badges/version/maiwj/rancher1.x-exporter.svg)](http://microbadger.com/images/maiwj/rancher1.x-exporter)&nbsp;[![](https://images.microbadger.com/badges/commit/maiwj/rancher1.x-exporter.svg)](http://microbadger.com/images/maiwj/rancher1.x-exporter.svg)

## References

### Rancher version supported

- [v1.6.12 and above](https://github.com/rancher/rancher/releases/tag/v1.6.12)

## How to use this image

### Running parameters

```bash
$ rancher-exporter -h
NAME:
   rancher_exporter - A simple server that scrapes Rancher 1.6 stats and exports them via HTTP for Prometheus consumption.

USAGE:
   rancher-exporter [global options] command [command options] [arguments...]

...

COMMANDS:
     help, h  Shows a list of commands or help for one command

GLOBAL OPTIONS:
  --listen_address value     The address of scraping the metrics (default: "0.0.0.0:9173") [$LISTEN_ADDRESS]
  --metric_path value        The path of exposing metrics (default: "/metrics") [$METRIC_PATH]
  --cattle_url value         The URL of Rancher Server API, e.g. http://127.0.0.1:8080 [$CATTLE_URL]
  --cattle_access_key value  The access key for Rancher API [$CATTLE_ACCESS_KEY]
  --cattle_secret_key value  The secret key for Rancher API [$CATTLE_SECRET_KEY]
  --log_level value          Set the logging level (default: "debug") [$LOG_LEVEL]
  --hide_sys                 Hide the system metrics [$HIDE_SYS]
  --help, -h                 show help
  --version, -v              print the version

```

### Start an instance

To start a container, use the following:

``` bash
$ docker run -d --name test-re -p 9173:9173 -e CATTLE_URL=<cattel_url> -e CATTLE_ACCESS_KEY=<cattel_ak> -e CATTLE_SECRET_KEY=<cattel_sk> maiwj/rancher1.x-exporter

```

## License

- Rancher is released under the [Apache License 2.0](https://github.com/rancher/rancher/blob/master/LICENSE)
- This image is released under the [MIT License](LICENSE)