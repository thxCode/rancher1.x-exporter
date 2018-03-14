# Metrics Sample

## [InfinityWorks](https://github.com/infinityworks/prometheus-rancher-exporter)

### Rancher agents state gauge

```
# HELP rancher_host_agent_state State of defined host agent as reported by the Rancher API
# TYPE rancher_host_agent_state gauge
rancher_host_agent_state{id, name, state=[activating|active|disconnected|disconnecting|finishing-reconnect|reconnected|reconnecting]} [1|0]

```

### Rancher host state gauge

```
# HELP rancher_host_state State of defined host as reported by the Rancher API
# TYPE rancher_host_state gauge
rancher_host_state{id, name, state=[activating|active|deactivating|error|erroring|inactive|provisioned|purged|purging|registering|removed|removing|requested|restoring|updating_active|updating_inactive]} [1|0]

```

### Rancher stack state gauge

```
# HELP rancher_stack_state State of defined stack as reported by Rancher
# TYPE rancher_stack_state gauge
rancher_stack_state{id, name, state=[activating|active|canceled_upgrade|canceling_upgrade|error|erroring|finishing_upgrade|removed|removing|requested|restarting|rolling_back|updating_active|upgraded|upgrading], system=[true|false]} [1|0]

```

### Rancher stack health gauge

```
# HELP rancher_stack_health_status HealthState of defined stack as reported by Rancher
# TYPE rancher_stack_health_status gauge
rancher_stack_health_status{health_state=[healthy|unhealthy], id, name, system=[true|false]} [1|0]

```

### Rancher service state gauge

```
# HELP rancher_service_state State of the service, as reported by the Rancher API
# TYPE rancher_service_state gauge
rancher_service_state{id, name, stack_id, stack_name, state=[activating|active|deactivating|error|erroring|inactive|provisioned|purged|purging|registering|removed|removing|requested|restoring|updating_active|updating_inactive]} [1|0]

```

### Rancher service health gauge

```
# HELP rancher_service_health_status HealthState of the service, as reported by the Rancher API
# TYPE rancher_service_health_status gauge
rancher_service_health_status{health_state=[healthy|unhealthy], id, name, stack_id, stack_name} [1|0]

```

### Rancher service scale gauge

* The scale is the number of containers which owned by service
* If the scale is equal to 0, perhaps belongs to the owned service has been closed

```
# HELP rancher_service_scale scale of defined service as reported by Rancher
# TYPE rancher_service_scale gauge
rancher_service_scale{name, stack_name} [1|0]

```

## Extending

* The `__rancher__` label value means masking the label key

### Rancher stacks bootstrap total

```
# HELP rancher_stacks_bootstrap_total Current total number of the bootstrap stacks in Rancher
# TYPE rancher_stacks_bootstrap_total counter
rancher_stacks_bootstrap_total{environment_name, name} 1

# HELP rancher_stacks_bootstrap_success_total Current total number of the healthy and active bootstrap stacks in Rancher
# TYPE rancher_stacks_bootstrap_success_total counter
rancher_stacks_bootstrap_success_total{environment_name, name} 1

# HELP rancher_stacks_bootstrap_error_total Current total number of the unhealthy or error bootstrap stacks in Rancher
# TYPE rancher_stacks_bootstrap_error_total counter
rancher_stacks_bootstrap_error_total{environment_name, name} 1

```

### Rancher services bootstrap total

```
# HELP rancher_services_bootstrap_total Current total number of the bootstrap services in Rancher
# TYPE rancher_services_bootstrap_total counter
rancher_services_bootstrap_total{environment_name, name, stack_name} 1

# HELP rancher_services_bootstrap_success_total Current total number of the healthy and active bootstrap services in Rancher
# TYPE rancher_services_bootstrap_success_total counter
rancher_services_bootstrap_success_total{environment_name, name, stack_name} 1

# HELP rancher_services_bootstrap_error_total Current total number of the unhealthy or error bootstrap services in Rancher
# TYPE rancher_services_bootstrap_error_total counter
rancher_services_bootstrap_error_total{environment_name, name, stack_name} 1

```

### Rancher instances bootstrap total

```
# HELP rancher_instances_bootstrap_total Current total number of the bootstrap instances in Rancher
# TYPE rancher_instances_bootstrap_total counter
rancher_instances_bootstrap_total{environment_name, name, service_name, stack_name} 1

# HELP rancher_instances_bootstrap_success_total Current total number of the healthy and active bootstrap instances in Rancher
# TYPE rancher_instances_bootstrap_success_total counter
rancher_instances_bootstrap_success_total{environment_name, name, service_name, stack_name} 1

# HELP rancher_instances_bootstrap_error_total Current total number of the unhealthy or error bootstrap instances in Rancher
# TYPE rancher_instances_bootstrap_error_total counter
rancher_instances_bootstrap_error_total{environment_name, name, service_name, stack_name} 1

```

### Rancher stacks initialization total

```
# HELP rancher_stacks_initialization_total Current total number of the initialization stacks in Rancher
# TYPE rancher_stacks_initialization_total counter
rancher_stacks_initialization_total{environment_name, name} 1

# HELP rancher_stacks_initialization_success_total Current total number of the healthy and active initialization stacks in Rancher
# TYPE rancher_stacks_initialization_success_total counter
rancher_stacks_initialization_success_total{environment_name, name} 1

# HELP rancher_stacks_initialization_error_total Current total number of the unhealthy or error initialization stacks in Rancher
# TYPE rancher_stacks_initialization_error_total counter
rancher_stacks_initialization_error_total{environment_name, name} 1

```

### Rancher services initialization total

```
# HELP rancher_services_initialization_total Current total number of the initialization services in Rancher
# TYPE rancher_services_initialization_total counter
rancher_services_initialization_total{environment_name, name, stack_name} 1

# HELP rancher_services_initialization_success_total Current total number of the healthy and active initialization services in Rancher
# TYPE rancher_services_initialization_success_total counter
rancher_services_initialization_success_total{environment_name, name, stack_name} 1

# HELP rancher_services_initialization_error_total Current total number of the unhealthy or error initialization services in Rancher
# TYPE rancher_services_initialization_error_total counter
rancher_services_initialization_error_total{environment_name, name, stack_name} 1

```

### Rancher instances initialization total

```
# HELP rancher_instances_initialization_total Current total number of the initialization instances in Rancher
# TYPE rancher_instances_initialization_total counter
rancher_instances_initialization_total{environment_name, name, service_name, stack_name} 1

# HELP rancher_instances_initialization_success_total Current total number of the healthy and active initialization instances in Rancher
# TYPE rancher_instances_initialization_success_total counter
rancher_instances_initialization_success_total{environment_name, name, service_name, stack_name} 1

# HELP rancher_instances_initialization_error_total Current total number of the unhealthy or error initialization instances in Rancher
# TYPE rancher_instances_initialization_error_total counter
rancher_instances_initialization_error_total{environment_name, name, service_name, stack_name} 1

```



### Rancher instances bootstrap milliseconds

```
# HELP rancher_instance_bootstrap_ms The bootstrap milliseconds of instances in Rancher
# TYPE rancher_instance_bootstrap_ms gauge
rancher_instance_bootstrap_ms{environment_nam, name, service_name, stack_name, system, type} ms

```

### Rancher heartbeat

* The metric value always be 1

```
# HELP rancher_stack_heartbeat The heartbeat of stacks in Rancher
# TYPE rancher_stack_heartbeat gauge
rancher_stack_heartbeat{environment_name, name, system, type} 1

# HELP rancher_service_heartbeat The heartbeat of services in Rancher
# TYPE rancher_service_heartbeat gauge
rancher_service_heartbeat{environment_name, name, stack_name, system, type} 1

# HELP rancher_instance_heartbeat The heartbeat of instances in Rancher
# TYPE rancher_instance_heartbeat gauge
rancher_instance_heartbeat{environment_name, name, service_name, stack_name, system, type} 1

```