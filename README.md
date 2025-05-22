# Phoenix-vNext: 3-Pipeline Cardinality Optimization System

Phoenix-vNext is an OpenTelemetry-based metrics collection and processing system that uses adaptive cardinality management with dynamic switching between optimization profiles based on metric volume and system performance.

## 🏗️ Architecture Overview

The system implements a 3-pipeline architecture for different cardinality optimization levels:

1. **Full Fidelity Pipeline** - Complete metrics collection baseline
2. **Optimized Pipeline** - Moderate cardinality reduction with aggregation  
3. **Experimental TopK Pipeline** - Advanced optimization using TopK sampling

## 📁 Project Structure

```
phoenix-vnext/
├── README.md                          # This file
├── docker-compose.yaml               # Main orchestration
├── CLAUDE.md                          # Claude Code guidance
├── .gitignore                         # Git ignore patterns
│
├── apps/                             # Application services
│   ├── synthetic-generator/          # Go-based metrics generator
│   └── control-actuator/             # Control plane actuator script
│
├── configs/
│   ├── otel/collectors/              # OpenTelemetry collector configurations
│   │   ├── main.yaml                 # Main collector (3 pipelines)
│   │   └── observer.yaml             # Observer collector
│   ├── monitoring/
│   │   ├── prometheus/               # Prometheus configs and rules
│   │   └── grafana/                  # Grafana datasources and dashboards
│   └── control/                      # Control plane configurations
│
├── docs/                             # Core documentation
│   ├── README.md                     # Documentation index
│   ├── ARCHITECTURE.md               # System design
│   └── TROUBLESHOOTING.md            # Problem resolution
│
├── scripts/                          # Environment initialization
└── data/                             # Runtime data (gitignored)
```

## 🚀 Quick Start

### Prerequisites

- Docker Desktop with WSL2 integration enabled
- 8GB+ RAM available for containers
- Ports 3000, 4318, 8888-8890, 9090, 13133-13134 available

### 1. Initialize Environment

```bash
# Clone and navigate to project
cd phoenix-reorganized

# Initialize environment (creates .env, data directories, control files)
./scripts/initialize-environment.sh

# Optional: Configure New Relic export (edit .env with your keys)
# NEW_RELIC_LICENSE_KEY_FULL="your_key_here"
```

### 2. Start the System

```bash
# Start all services
docker-compose up -d

# View logs
docker-compose logs -f otelcol-main
docker-compose logs -f otelcol-observer
```

### 3. Access Monitoring

- **Grafana Dashboard**: http://localhost:3000 (admin/admin)
- **Prometheus**: http://localhost:9090
- **Main Collector Metrics**: http://localhost:8888/metrics
- **Observer Metrics**: http://localhost:9888/metrics

## 📊 System Components

### Core Services

| Service | Description | Ports |
|---------|-------------|-------|
| **otelcol-main** | Main collector with 3 pipelines | 4318, 8888-8890, 13133 |
| **otelcol-observer** | Control plane observer | 9888, 13134 |
| **control-loop-actuator** | Adaptive controller script | - |
| **synthetic-metrics-generator** | Load generator | - |
| **prometheus** | Metrics storage | 9090 |
| **grafana** | Visualization | 3000 |

### Load Generators

| Service | Description | Resource Limits |
|---------|-------------|-----------------|
| **stress-ng-cpu-heavy** | CPU-intensive workload | 2 CPU, 1GB RAM |
| **stress-ng-io-heavy** | I/O-intensive workload | 1 CPU, 512MB RAM |

## 🎛️ Adaptive Control System

The observer uses a PID-like control algorithm that:

- Monitors metric cardinality and system performance
- Automatically switches between optimization profiles:
  - **Conservative**: < 15,000 time series
  - **Balanced**: 15,000 - 25,000 time series  
  - **Aggressive**: > 25,000 time series
- Updates control signals in real-time
- Maintains stability with configurable transition periods

## 🔧 Configuration

### Environment Variables

Key variables in `.env`:

```bash
# Control thresholds
TARGET_OPTIMIZED_PIPELINE_TS_COUNT=20000
THRESHOLD_OPTIMIZATION_CONSERVATIVE_MAX_TS=15000
THRESHOLD_OPTIMIZATION_AGGRESSIVE_MIN_TS=25000

# Resource limits
OTELCOL_MAIN_MEMORY_LIMIT_MIB="1024"
OTELCOL_MAIN_GOMAXPROCS="1"

# Synthetic load
SYNTHETIC_PROCESS_COUNT_PER_HOST=250
SYNTHETIC_HOST_COUNT=3
```

### Control Signals

The system uses dynamic control files in `configs/control/`:
- `optimization_mode.yaml` - Current optimization state
- `optimization_mode_template.yaml` - Template for control file

## 🔍 Monitoring & Troubleshooting

### Health Checks

```bash
# Check service health
docker-compose ps

# View specific service logs
docker-compose logs -f [service-name]

# Check collector endpoints
curl http://localhost:13133  # Main collector health
curl http://localhost:13134  # Observer health
```

### Key Metrics

Monitor these metrics in Grafana:
- `phoenix_pipeline_output_cardinality_estimate` - Pipeline cardinality
- `otelcol_processor_batch_batch_send_size` - Batch processing
- `process_memory_usage` - Process memory consumption
- `process_cpu_time` - CPU utilization

## 🛠️ Development

### Testing Synthetic Data

```bash
# Generate synthetic metrics
docker-compose up synthetic-metrics-generator

# Update control signals manually
./scripts/update-control-file.sh
```

### Adding New Processors

1. Add processor config to `configs/otel/processors/`
2. Include in pipeline via `configs/otel/collectors/main.yaml`
3. Update documentation

### Scaling Configuration

Adjust resource limits in `docker-compose.yaml` and corresponding environment variables in `.env`.

## 📝 License

This project is part of the Phoenix-vNext Ultimate Stack and follows the same licensing terms.