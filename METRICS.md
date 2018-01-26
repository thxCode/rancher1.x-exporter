# Metrics Sample

## Counter Metrics

### Rancher stacks total counter

```

# HELP rancher_stack_bootstrap_total Current total number of the started stacks in Rancher.
# TYPE rancher_stack_bootstrap_total counter
rancher_stack_bootstrap_total{environment_id, environment_name, id, name, type, system=[true|false]} [1|0]

# HELP rancher_stack_failure_total Current total number of the failure stacks in Rancher.
# TYPE rancher_stack_failure_total counter
rancher_stack_failure_total{environment_id, environment_name, id, name, type, system=[true|false]} [1|0]

```

### Rancher services total counter

```
# HELP rancher_service_bootstrap_total Current total number of the started services in Rancher.
# TYPE rancher_service_bootstrap_total counter
rancher_service_bootstrap_total{environment_id, environment_name, stack_id, stack_name, id, name, type, system=[true|false]} [1|0]

# HELP rancher_service_failure_total Current total number of the failure services in Rancher.
# TYPE rancher_service_failure_total counter
rancher_service_failure_total{environment_id, environment_name, id, name, type, system=[true|false]} [1|0]

```

### Rancher instances total counter

* Container info can filter by 'type="container"'
* health_state only can be detected if and only if health check is enabled on Rancher

```
# HELP rancher_instance_bootstrap_total Current total number of the started containers in Rancher.
# TYPE rancher_instance_bootstrap_total counter
rancher_instance_bootstrap_total{environment_id, environment_name, stack_id, stack_name, service_id, service_name, id, name, type} [1|0]

# HELP rancher_instance_failure_total Current total number of the failure containers in Rancher.
# TYPE rancher_instance_failure_total counter
rancher_instance_failure_total{environment_id, environment_name, stack_id, stack_name, service_id, service_name, id, name, type} [1|0]

```


## Gauge Metrics

## Rancher instances startup gauge

* Container info can filter by 'type="container"'

```

# HELP rancher_instance_startup_ms The startup milliseconds of container in Rancher.
# TYPE rancher_instance_startup_ms gauge
rancher_instance_startup_ms{environment_id, environment_name, stack_id, stack_name, service_id, service_name, id, name, type} milliseconds
```

## Rancher agents state gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_host_agent_state State of defined host agent as reported by the Rancher API
# TYPE rancher_host_agent_state gauge
rancher_host_agent_state{id, name, state=[activating|active|disconnected|disconnecting|finishing-reconnect|reconnected|reconnecting]} [1|0]

```

## Rancher host state gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_host_state State of defined host as reported by the Rancher API
# TYPE rancher_host_state gauge
rancher_host_state{id, name, state=[activating|active|deactivating|error|erroring|inactive|provisioned|purged|purging|registering|removed|removing|requested|restoring|updating_active|updating_inactive]} [1|0]

```

## Rancher stack state gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_stack_state State of defined stack as reported by Rancher
# TYPE rancher_stack_state gauge
rancher_stack_state{id, name, state=[activating|active|canceled_upgrade|canceling_upgrade|error|erroring|finishing_upgrade|removed|removing|requested|restarting|rolling_back|updating_active|upgraded|upgrading], system=[true|false]} [1|0]

```

## Rancher stack health gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_stack_health_status HealthState of defined stack as reported by Rancher
# TYPE rancher_stack_health_status gauge
rancher_stack_health_status{health_state=[healthy|unhealthy], id, name, system=[true|false]} [1|0]

```

## Rancher service state gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_service_state State of the service, as reported by the Rancher API
# TYPE rancher_service_state gauge
rancher_service_state{id, name, stack_id, stack_name, state=[activating|active|deactivating|error|erroring|inactive|provisioned|purged|purging|registering|removed|removing|requested|restoring|updating_active|updating_inactive]} [1|0]

```

## Rancher service health gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

```
# HELP rancher_service_health_status HealthState of the service, as reported by the Rancher API
# TYPE rancher_service_health_status gauge
rancher_service_health_status{health_state=[healthy|unhealthy], id, name, stack_id, stack_name} [1|0]

```

## Rancher service scale gauge [infinityworks](https://github.com/infinityworks/prometheus-rancher-exporter)

* The scale is the number of containers which owned by service
* If the scale is equal to 0, perhaps belongs to the owned service has been closed

```
# HELP rancher_service_scale scale of defined service as reported by Rancher
# TYPE rancher_service_scale gauge
rancher_service_scale{name, stack_name} [1|0]

```