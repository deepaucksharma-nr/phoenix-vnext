#!/usr/bin/env bash
# Phoenix-vNext Debug Information Collection Script
# Collects comprehensive debug information for troubleshooting

set -euo pipefail

# Configuration
DEBUG_DIR="debug-$(date +%Y%m%d-%H%M%S)"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "=== Phoenix-vNext Debug Information Collection ==="
echo "Creating debug package in: $DEBUG_DIR"
echo

# Create debug directory
mkdir -p "$DEBUG_DIR"
cd "$DEBUG_DIR"

# Function to safely run commands and capture output
safe_exec() {
    local cmd="$1"
    local output_file="$2"
    echo "Collecting: $cmd"
    
    {
        echo "Command: $cmd"
        echo "Timestamp: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
        echo "----------------------------------------"
        eval "$cmd" 2>&1 || echo "Command failed with exit code $?"
        echo
    } > "$output_file"
}

# Function to safely copy files
safe_copy() {
    local src="$1"
    local dst="$2"
    if [ -f "$src" ] || [ -d "$src" ]; then
        cp -r "$src" "$dst" 2>/dev/null || echo "Failed to copy $src"
    else
        echo "File not found: $src" > "$dst.missing"
    fi
}

echo "1. System Information"
safe_exec "uname -a" "01-system-info.txt"
safe_exec "docker --version" "02-docker-version.txt"
safe_exec "docker-compose --version" "03-docker-compose-version.txt"

echo "2. Docker Container Status"
safe_exec "docker-compose ps" "04-container-status.txt"
safe_exec "docker-compose top" "05-container-processes.txt"
safe_exec "docker stats --no-stream" "06-container-stats.txt"

echo "3. Container Logs"
mkdir -p logs
for service in otelcol-main otelcol-observer control-loop-actuator synthetic-metrics-generator prometheus grafana; do
    safe_exec "docker-compose logs --tail=200 $service" "logs/$service.log"
done

echo "4. Health Check Status"
safe_exec "curl -s http://localhost:13133" "07-main-collector-health.txt"
safe_exec "curl -s http://localhost:13134" "08-observer-health.txt"
safe_exec "curl -s http://localhost:9090/-/healthy" "09-prometheus-health.txt"
safe_exec "curl -s http://localhost:3000/api/health" "10-grafana-health.txt"

echo "5. Metrics Endpoints"
mkdir -p metrics
safe_exec "curl -s http://localhost:8888/metrics" "metrics/main-collector-8888.txt"
safe_exec "curl -s http://localhost:8889/metrics" "metrics/optimized-pipeline-8889.txt"
safe_exec "curl -s http://localhost:8890/metrics" "metrics/experimental-pipeline-8890.txt"
safe_exec "curl -s http://localhost:9888/metrics" "metrics/observer-kpis-9888.txt"

echo "6. Prometheus Targets and Config"
safe_exec "curl -s http://localhost:9090/api/v1/targets" "11-prometheus-targets.txt"
safe_exec "curl -s http://localhost:9090/api/v1/status/config" "12-prometheus-config.txt"
safe_exec "curl -s http://localhost:9090/api/v1/rules" "13-prometheus-rules.txt"

echo "7. Sample Prometheus Queries"
mkdir -p queries
safe_exec "curl -s 'http://localhost:9090/api/v1/query?query=up'" "queries/up-status.txt"
safe_exec "curl -s 'http://localhost:9090/api/v1/query?query=phoenix_opt_final_output_phoenix_optimised_output_ts_active'" "queries/optimized-ts-count.txt"
safe_exec "curl -s 'http://localhost:9090/api/v1/query?query=phoenix:cost_reduction_ratio'" "queries/cost-reduction-ratio.txt"

echo "8. Configuration Files"
mkdir -p configs
cd "$PROJECT_ROOT"
safe_copy "configs/" "$DEBUG_DIR/configs/"
safe_copy ".env" "$DEBUG_DIR/configs/env-file.txt"
safe_copy ".env.template" "$DEBUG_DIR/configs/env-template.txt"
safe_copy "docker-compose.yaml" "$DEBUG_DIR/configs/docker-compose.yaml"

echo "9. Control System State"
cd "$DEBUG_DIR"
safe_copy "$PROJECT_ROOT/configs/control/optimization_mode.yaml" "14-current-optimization-mode.yaml"
safe_exec "ls -la $PROJECT_ROOT/configs/control/" "15-control-directory-listing.txt"

echo "10. Grafana Dashboard Status"
safe_exec "curl -s -u admin:admin http://localhost:3000/api/dashboards/home" "16-grafana-dashboards.txt"
safe_exec "curl -s -u admin:admin http://localhost:3000/api/datasources" "17-grafana-datasources.txt"

echo "11. Process and Resource Information"
safe_exec "ps aux | grep -E '(otelcol|prometheus|grafana)'" "18-relevant-processes.txt"
safe_exec "netstat -tlnp | grep -E ':(3000|8888|8889|8890|9090|9888|13133|13134)'" "19-port-usage.txt"
safe_exec "df -h" "20-disk-usage.txt"
safe_exec "free -h" "21-memory-usage.txt"

echo "12. Network Connectivity"
mkdir -p connectivity
safe_exec "curl -I http://localhost:3000" "connectivity/grafana-connection.txt"
safe_exec "curl -I http://localhost:9090" "connectivity/prometheus-connection.txt"
safe_exec "curl -I http://localhost:8888" "connectivity/main-collector-connection.txt"

echo "13. Environment Variables"
safe_exec "docker-compose config" "22-docker-compose-resolved.txt"

# Create summary file
{
    echo "=== Phoenix-vNext Debug Package Summary ==="
    echo "Generated: $(date -u +"%Y-%m-%dT%H:%M:%SZ")"
    echo "Project Root: $PROJECT_ROOT"
    echo "Debug Directory: $DEBUG_DIR"
    echo
    echo "=== Key Files ==="
    echo "• Container Status: 04-container-status.txt"
    echo "• Main Collector Health: 07-main-collector-health.txt"
    echo "• Prometheus Targets: 11-prometheus-targets.txt"
    echo "• Current Control Mode: 14-current-optimization-mode.yaml"
    echo "• Container Logs: logs/"
    echo "• Metrics Snapshots: metrics/"
    echo "• Configuration Files: configs/"
    echo
    echo "=== Usage ==="
    echo "1. Review container status and health checks"
    echo "2. Check logs/ directory for service errors"
    echo "3. Verify Prometheus targets are up"
    echo "4. Examine metrics endpoints for data flow"
    echo "5. Review control system state"
    echo
    echo "=== Next Steps ==="
    echo "• Run health-check.sh for current status"
    echo "• Run validate-data-flow.sh for metrics validation"
    echo "• Check Grafana at http://localhost:3000"
    echo "• Monitor Prometheus at http://localhost:9090"
} > "00-DEBUG-SUMMARY.txt"

cd "$PROJECT_ROOT"
echo
echo "Debug information collected in: $DEBUG_DIR"
echo "Review 00-DEBUG-SUMMARY.txt for an overview"
echo "Archive with: tar -czf phoenix-debug-$(date +%Y%m%d-%H%M%S).tar.gz $DEBUG_DIR"