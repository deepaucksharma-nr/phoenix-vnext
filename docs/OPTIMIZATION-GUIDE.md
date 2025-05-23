# Phoenix v3 Optimization Guide

This guide consolidates all optimization strategies for the Phoenix-vNext system.

## Quick Start

1. **Initialize environment**: `./scripts/initialize-environment.sh`
2. **Start the stack**: `docker-compose up -d`
3. **Monitor cardinality**: `./scripts/monitoring/cardinality-monitor.sh`
4. **Validate functionality**: `./scripts/validate-functional.sh`

## Architecture Overview

Phoenix-vNext implements a 3-pipeline cardinality optimization system:

- **Full Fidelity Pipeline**: Complete metrics baseline (port 8888)
- **Optimized Pipeline**: Balanced cardinality reduction (port 8889)  
- **Experimental Pipeline**: Aggressive optimization (port 8890)

The Observer (port 9888) monitors metrics and the Control Actuator adjusts optimization profiles based on cardinality thresholds.

## Configuration Optimization

### Cardinality Reduction Techniques

1. **Priority-based Filtering**
   - Critical services: Full metrics retained
   - Infrastructure: Moderate reduction
   - System processes: Aggressive filtering

2. **Attribute Stripping**
   - Remove PIDs universally
   - Keep command lines for debugging critical services
   - Strip all attributes from low-priority metrics

3. **Smart Aggregation**
   - Aggregate by host for system metrics
   - Preserve service-level granularity for business metrics

### Adaptive Control Thresholds

- **Conservative**: < 15,000 time series
- **Balanced**: 15,000 - 25,000 time series
- **Aggressive**: > 25,000 time series

## Functional Testing

### Core Validation
```bash
# Full validation suite
./scripts/validate-functional.sh

# Specific tests
./scripts/testing/functional-test.sh
```

### Performance Testing
```bash
# Quick configuration test
./scripts/testing/test-configurations.sh quick

# Benchmark configurations
./scripts/testing/benchmark-configs.sh
```

### Monitoring
```bash
# Real-time cardinality monitoring
./scripts/monitoring/cardinality-monitor.sh monitor

# One-time analysis
./scripts/monitoring/cardinality-monitor.sh analyze

# Health check
./scripts/monitoring/health-check.sh
```

## Best Practices

1. **Start Conservative**: Begin with high cardinality limits and reduce gradually
2. **Monitor Impact**: Track alert accuracy and troubleshooting effectiveness
3. **Preserve Critical Signals**: Never filter essential business metrics
4. **Test Changes**: Validate all configuration changes with functional tests
5. **Document Decisions**: Record why specific optimizations were applied

## Troubleshooting

See [TROUBLESHOOTING.md](TROUBLESHOOTING.md) for common issues and solutions.

## Results

Typical cardinality reduction achieved:
- Optimized pipeline: 20-30% reduction
- Experimental pipeline: 60-80% reduction
- No loss of critical business signals
- Adaptive control maintains stability