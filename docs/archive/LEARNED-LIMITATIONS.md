# Phoenix-vNext: Learned Limitations & Observations

This document captures key learnings from running the Phoenix-vNext system with OpenTelemetry Collector v0.103.1.

## 1. Configuration Source Limitations

### Issue: config_sources Not Supported
The OpenTelemetry Collector Contrib v0.103.1 does not support the `config_sources` feature that would allow dynamic configuration reloading from files.

**Attempted Configuration:**
```yaml
config_sources:
  ctlfile_optimization_mode:
    path: /etc/otelcol/control/optimization_mode.yaml
    watch: true
    reload_delay: 10s
```

**Result:** 
```
Error: '' has invalid keys: config_sources
```

### Impact on Adaptive Routing
Without config_sources, the collector cannot dynamically read the control file to adjust pipeline routing based on optimization profiles. This means:
- Cannot implement true adaptive pipeline selection at runtime
- All pipelines must run continuously 
- Control must be implemented through filtering within pipelines

### Workaround
Instead of conditional routing, all pipelines receive all metrics and use filtering to control what they process.

## 2. Transform Processor Syntax Issues

### Issue: OTTL Context Limitations
The transform processor has strict requirements for OTTL (OpenTelemetry Transformation Language) syntax:

**Incorrect:**
```yaml
- context: datapoint
  statements:
    - set(value, Double(1.0)) where metric.name == "phoenix_full_output_ts_active"
```

**Error:**
```
segment "value" from path "value" is not a valid path nor a valid OTTL keyword for the DataPoint context
```

### Issue: Metric Context Restrictions
Cannot access certain fields in filter conditions:

**Incorrect:**
```yaml
filter/optimised_selection:
  metrics:
    metric:
      - 'resource.attributes["phoenix.priority"] == "high" and metric.data_points != nil'
```

**Error:**
```
segment "metric" from path "metric.data_points" is not a valid path nor a valid OTTL keyword for the Metric context
```

## 3. Logging Exporter Configuration

### Issue: Incompatible Settings
The logging exporter changed its configuration between versions:

**Incorrect:**
```yaml
logging/debug_sampled:
  loglevel: info
  verbosity: basic
```

**Error:**
```
'loglevel' and 'verbosity' are incompatible. Use only 'verbosity' instead
```

**Correct:**
```yaml
logging/debug_sampled:
  verbosity: basic
  sampling_initial: 2
  sampling_thereafter: 1000
```

## 4. CumulativeToDelta Processor

### Issue: Missing match_type
The processor requires explicit match type when metrics are specified:

**Incorrect:**
```yaml
cumulativetodelta:
  metrics:
    - process.cpu.time
```

**Correct:**
```yaml
cumulativetodelta:
  include:
    metrics:
      - process.cpu.time
    match_type: strict
```

## 5. Health Check Limitations

### Issue: Missing curl in Container
The OpenTelemetry Collector container doesn't include curl, making HTTP health checks fail:

**Incorrect:**
```yaml
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost:13133"]
```

**Error:**
```
exec: "curl": executable file not found in $PATH
```

**Workaround:**
```yaml
healthcheck:
  test: ["CMD", "/otelcol-contrib", "--version"]
```

## 6. Environment Variable Syntax

### Issue: Default Values in Variable Names
Cannot use shell-style default values in environment variable references:

**Incorrect:**
```yaml
- set(attributes["phoenix.optimisation_profile"], "${env:OPTIMIZATION_PROFILE:-conservative}")
```

**Error:**
```
environment variable "OPTIMIZATION_PROFILE:-conservative" has invalid name: must match regex ^[a-zA-Z_][a-zA-Z0-9_]*$
```

**Workaround:**
Use static values or ensure environment variables are always set.

## Key Takeaways

1. **Version Compatibility**: Always check the specific OpenTelemetry Collector version documentation for supported features
2. **OTTL Syntax**: Refer to the official OTTL context documentation for valid paths and operations
3. **Simplified Configuration**: Start with a minimal working configuration and add complexity incrementally
4. **Testing**: Test configuration changes in isolation before integrating into the full system
5. **Monitoring**: Use prometheus metrics and logging exporters to observe actual behavior

## Recommendations for Production

1. **Use Config Reloading**: Consider upgrading to a collector version that supports config_sources or use external configuration management
2. **Implement Control at Application Level**: Since dynamic routing isn't available, implement cardinality control in the application or through a proxy
3. **Monitor Resource Usage**: Without dynamic pipeline control, monitor memory and CPU usage closely
4. **Plan for Scale**: Design assuming all pipelines run continuously rather than adaptively