# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Phoenix-vNext is a 3-Pipeline Cardinality Optimization System for OpenTelemetry metrics collection and processing. The system uses adaptive cardinality management with dynamic switching between optimization profiles (conservative, balanced, aggressive) based on metric volume and system performance through a threshold-based control algorithm with hysteresis.

## Architecture

### Core System Components
- **Main Collector** (`otelcol-main`): Runs 3 parallel pipelines with different cardinality optimization levels
- **Observer Collector** (`otelcol-observer`): Control plane that monitors pipeline metrics and system performance
- **Control Actuator** (`control-loop-actuator`): Bash script implementing threshold-based adaptive control logic with hysteresis and stability periods
- **Synthetic Generator** (`synthetic-metrics-generator`): Go-based load generator for testing and benchmarking

### Pipeline Architecture
The system operates 3 distinct pipelines in parallel:
1. **Full Fidelity Pipeline** (`pipeline_full_fidelity`) - Complete metrics baseline without optimization
2. **Optimized Pipeline** (`pipeline_optimised`) - Moderate cardinality reduction with configurable aggregation
3. **Experimental TopK Pipeline** (`pipeline_experimental_topk`) - Advanced optimization using TopK sampling techniques

### Adaptive Control System
- Observer monitors `phoenix_observer_kpi_store_phoenix_pipeline_output_cardinality_estimate` metrics
- Control actuator applies threshold-based switching with 10% hysteresis buffers:
  - Conservative: < 15,000 time series
  - Balanced: 15,000 - 25,000 time series  
  - Aggressive: > 25,000 time series
- Control signals written atomically to `configs/control/optimization_mode.yaml` using `yq` with file locking
- 120-second stability period prevents rapid profile oscillation
- Lock file mechanism prevents concurrent control script execution

## Development Commands

### Environment Setup
```bash
# Initialize environment (creates data dirs, control files, .env from template)
./scripts/initialize-environment.sh

# Generate checksums for configuration validation
sha256sum configs/otel/collectors/*.yaml configs/control/*template.yaml > CHECKSUMS.txt
```

### Running the System
```bash
# Start full stack with all services
docker-compose up -d

# Start specific services
docker-compose up -d otelcol-main otelcol-observer prometheus grafana

# View logs from key services
docker-compose logs -f otelcol-main
docker-compose logs -f control-loop-actuator

# Stop services
docker-compose down
```

### Development & Testing
```bash
# Rebuild and restart a specific service
docker-compose build synthetic-metrics-generator
docker-compose up -d synthetic-metrics-generator

# Validate docker-compose configuration
docker-compose config

# Generate synthetic load for testing
docker-compose up synthetic-metrics-generator

# Monitor control signal changes
watch cat configs/control/optimization_mode.yaml

# Check service health
curl http://localhost:13133  # Main collector health
curl http://localhost:13134  # Observer health
```

## Configuration Architecture

### OpenTelemetry Configurations
- `configs/otel/collectors/main.yaml`: Core collector with 3-pipeline configuration, includes processor chains and exporters
- `configs/otel/collectors/observer.yaml`: Monitoring collector that exposes KPI metrics for control decisions
- `configs/otel/processors/common_intake_processors.yaml`: Shared processor configurations used across pipelines

### Control System
- `configs/control/optimization_mode.yaml`: Dynamic control file modified by actuator, read by main collector via config_sources
- `configs/control/optimization_mode_template.yaml`: Template defining control file schema and default values

### Monitoring Stack
- `configs/monitoring/prometheus/prometheus.yaml`: Prometheus scrape configuration for all collector endpoints
- `configs/monitoring/prometheus/rules/phoenix_rules.yaml`: Recording rules and alerts for optimization metrics
- `configs/monitoring/grafana/`: Datasource configuration and dashboard provisioning with functional dashboards

## Key Environment Variables

Critical variables in `.env`:
```bash
# Control thresholds for adaptive switching
TARGET_OPTIMIZED_PIPELINE_TS_COUNT=20000
THRESHOLD_OPTIMIZATION_CONSERVATIVE_MAX_TS=15000
THRESHOLD_OPTIMIZATION_AGGRESSIVE_MIN_TS=25000

# Resource constraints
OTELCOL_MAIN_MEMORY_LIMIT_MIB="1024"
OTELCOL_MAIN_GOMAXPROCS="1"

# Control loop timing
ADAPTIVE_CONTROLLER_INTERVAL_SECONDS=60
ADAPTIVE_CONTROLLER_STABILITY_SECONDS=120

# Load generation
SYNTHETIC_PROCESS_COUNT_PER_HOST=250
SYNTHETIC_HOST_COUNT=3
SYNTHETIC_METRIC_EMIT_INTERVAL_S=15

# New Relic export (disabled by default for local testing)
ENABLE_NR_EXPORT_FULL="false"
ENABLE_NR_EXPORT_OPTIMISED="false"
ENABLE_NR_EXPORT_EXPERIMENTAL="false"
```

## Monitoring & Access Points

### Service Endpoints
- **Grafana Dashboard**: http://localhost:3000 (admin/admin) - Two functional dashboards: Phoenix Overview and Adaptive Control Loop
- **Prometheus**: http://localhost:9090 - Includes recording rules for cost reduction metrics and signal preservation analysis
- **Main Collector Metrics**: http://localhost:8888/metrics (full pipeline + collector telemetry)
- **Optimized Pipeline**: http://localhost:8889/metrics 
- **Experimental Pipeline**: http://localhost:8890/metrics
- **Observer Metrics**: http://localhost:9888/metrics (KPIs for control decisions)

### Health Checks
- Main Collector: http://localhost:13133
- Observer Collector: http://localhost:13134
- Docker health checks configured with 20s intervals, 3 retries

## Control Flow & Data Paths

1. **Metrics Ingestion**: Synthetic generator → Main collector OTLP endpoint (4318)
2. **Pipeline Processing**: 3 parallel processing chains with different optimization levels
3. **Metrics Export**: Each pipeline exports to dedicated Prometheus endpoints (8888-8890)
4. **Monitoring**: Observer scrapes main collector metrics and exposes KPIs (9888)
5. **Control Loop**: Actuator queries observer metrics → calculates profile → updates control file
6. **Adaptation**: Main collector reads control file changes → adjusts pipeline behavior

## Development Patterns

### Adding New Processors
1. Create processor config in `configs/otel/processors/`
2. Include in pipeline via `configs/otel/collectors/main.yaml`
3. Test with synthetic load and monitor cardinality impact

### Modifying Control Logic
1. Update thresholds in `.env` file
2. Modify logic in `apps/control-actuator/update-control-file.sh`
3. Test profile transitions using different load patterns

### Performance Tuning
- Monitor resource usage: `docker-compose top`
- Adjust memory limits via environment variables
- Scale synthetic load via `SYNTHETIC_*` variables
- Profile collector performance via pprof endpoints (1777, 1778)

## File Organization

- **`apps/`**: Self-contained services with Dockerfiles and source code
- **`configs/`**: Technology-grouped configurations (otel, monitoring, control)
- **`scripts/`**: Operational utilities and environment setup
- **`data/`**: Persistent storage directories (gitignored)