# Phoenix-vNext System Test Plan

This document outlines the test procedures to verify that all architectural fixes are working correctly.

## Prerequisites

1. Docker and Docker Compose installed
2. Environment initialized: `./scripts/initialize-environment.sh`
3. Configuration validated: `./scripts/validate-config.sh`

## Test Scenarios

### 1. Basic System Startup Test

**Objective**: Verify all services start correctly with fixed configurations

```bash
# Start the system
docker-compose up -d

# Wait for services to initialize (30 seconds)
sleep 30

# Check service health
docker-compose ps
curl -f http://localhost:13133/health  # Main collector
curl -f http://localhost:13134/health  # Observer collector
curl -f http://localhost:9090/-/healthy # Prometheus
curl -f http://localhost:3000/api/health # Grafana
```

**Expected Results**:
- All services show as "healthy"
- No error logs in first 60 seconds
- Control file created at `configs/control/optimization_mode.yaml`

### 2. Adaptive Routing Test

**Objective**: Verify pipelines activate based on optimization profile

```bash
# Check initial state (should be conservative)
cat configs/control/optimization_mode.yaml | grep optimization_profile

# Monitor pipeline metrics
curl -s http://localhost:8888/metrics | grep phoenix_full_output_ts_active
curl -s http://localhost:8889/metrics | grep phoenix_optimised_output_ts_active
curl -s http://localhost:8890/metrics | grep phoenix_experimental_output_ts_active
```

**Test Profile Switching**:
```bash
# Manually update profile to test routing
docker exec phoenix-vnext_control-loop-actuator_1 yq eval '.optimization_profile = "balanced"' -i /app/control_signals/optimization_mode.yaml

# Wait for config reload (15 seconds)
sleep 15

# Verify optimised pipeline is now active
curl -s http://localhost:8889/metrics | grep phoenix_optimised_output_ts_active
```

**Expected Results**:
- Conservative mode: Only full pipeline has metrics
- Balanced mode: Full + Optimised pipelines have metrics
- Aggressive mode: All three pipelines have metrics

### 3. Control Loop Function Test

**Objective**: Verify automatic profile switching based on cardinality

```bash
# Monitor control loop logs
docker-compose logs -f control-loop-actuator &

# Simulate high cardinality by scaling synthetic load
docker-compose scale synthetic-metrics-generator=3

# Wait for control loop iterations (2-3 minutes)
sleep 180

# Check if profile changed
cat configs/control/optimization_mode.yaml | grep -E "optimization_profile|trigger_reason"
```

**Expected Results**:
- Control loop detects increased cardinality
- Profile switches from conservative → balanced → aggressive
- Trigger reasons logged correctly
- No lock contention errors

### 4. Resource Isolation Test

**Objective**: Verify memory limiters work independently

```bash
# Monitor memory usage per pipeline
docker stats --no-stream --format "table {{.Container}}\t{{.MemUsage}}\t{{.MemLimit}}"

# Generate load spike
docker exec phoenix-vnext_synthetic-metrics-generator_1 kill -USR1 1

# Monitor for memory limit violations
docker-compose logs otelcol-main | grep -i "memory limit"
```

**Expected Results**:
- Each pipeline stays within its memory limit
- No cascade failures between pipelines
- Memory limiter logs show independent operation

### 5. Metric Consistency Test

**Objective**: Verify observer correctly tracks pipeline cardinality

```bash
# Query observer metrics
curl -s http://localhost:9888/metrics | grep phoenix_observer_kpi_store_phoenix_pipeline_output_cardinality_estimate

# Compare with actual pipeline metrics
for port in 8888 8889 8890; do
  echo "Pipeline on port $port:"
  curl -s http://localhost:$port/metrics | grep -c "^phoenix_" || echo "0"
done
```

**Expected Results**:
- Observer reports matching cardinality for each pipeline
- Metric names properly transformed
- No "metric not found" errors in observer logs

### 6. Lock Mechanism Test

**Objective**: Verify control script lock prevents concurrent execution

```bash
# Try to run control script manually while it's running
docker exec phoenix-vnext_control-loop-actuator_1 /app/update-control-file.sh &
docker exec phoenix-vnext_control-loop-actuator_1 /app/update-control-file.sh &

# Check for lock messages
docker-compose logs control-loop-actuator | grep -i lock
```

**Expected Results**:
- Second execution waits for lock
- No corrupted control file
- Lock properly released after execution

### 7. Security Test

**Objective**: Verify security improvements

```bash
# Check process ownership
docker exec phoenix-vnext_control-loop-actuator_1 ps aux | grep update-control

# Verify health checks
docker inspect phoenix-vnext_control-loop-actuator_1 | jq '.State.Health'

# Test Grafana requires authentication
curl -I http://localhost:3000/api/dashboards/home
```

**Expected Results**:
- Control actuator runs as non-root user (UID 1000)
- Health check shows "healthy" status
- Grafana returns 401 without credentials

## Cleanup

```bash
# Stop all services
docker-compose down

# Remove test data
rm -rf data/*

# Reset control file
rm -f configs/control/optimization_mode.yaml
```

## Troubleshooting

### Common Issues:

1. **Services not starting**: Check docker logs and ensure ports are free
2. **Metrics not appearing**: Wait for scrape interval (15s) and check endpoints
3. **Control loop not switching**: Verify thresholds in .env match load levels
4. **Lock errors**: Ensure phoenix_lock_volume is properly created

### Debug Commands:

```bash
# View all logs
docker-compose logs

# Check specific service
docker-compose logs -f otelcol-main

# Inspect control file
watch -n 5 'cat configs/control/optimization_mode.yaml | yq'

# Monitor all metrics endpoints
for port in 8888 8889 8890 9888; do
  echo "=== Port $port ==="
  curl -s http://localhost:$port/metrics | grep -E "(phoenix_|up{)" | head -5
done
```