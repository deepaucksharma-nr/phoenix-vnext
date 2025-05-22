#!/usr/bin/env bash
# Phoenix-vNext Health Check Script
# Checks the health of all system components

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Health check functions
check_service_health() {
    local service_name="$1"
    local health_url="$2"
    
    echo -n "Checking $service_name... "
    if curl -sf "$health_url" >/dev/null 2>&1; then
        echo -e "${GREEN}✓ Healthy${NC}"
        return 0
    else
        echo -e "${RED}✗ Unhealthy${NC}"
        return 1
    fi
}

check_prometheus_target() {
    local target_name="$1"
    local expected_targets="$2"
    
    echo -n "Checking Prometheus target $target_name... "
    local active_targets=$(curl -s "http://localhost:9090/api/v1/targets" | grep -o "\"health\":\"up\"" | wc -l)
    
    if [ "$active_targets" -ge "$expected_targets" ]; then
        echo -e "${GREEN}✓ $active_targets/$expected_targets targets up${NC}"
        return 0
    else
        echo -e "${YELLOW}⚠ Only $active_targets/$expected_targets targets up${NC}"
        return 1
    fi
}

echo "=== Phoenix-vNext System Health Check ==="
echo

# Check core services
echo "Core Services:"
check_service_health "Main Collector" "http://localhost:13133"
check_service_health "Observer Collector" "http://localhost:13134"
check_service_health "Prometheus" "http://localhost:9090/-/healthy"
check_service_health "Grafana" "http://localhost:3000/api/health"

echo

# Check metrics endpoints
echo "Metrics Endpoints:"
check_service_health "Main Collector Metrics" "http://localhost:8888/metrics"
check_service_health "Optimized Pipeline" "http://localhost:8889/metrics"
check_service_health "Experimental Pipeline" "http://localhost:8890/metrics"
check_service_health "Observer Metrics" "http://localhost:9888/metrics"

echo

# Check Prometheus targets
echo "Prometheus Targets:"
check_prometheus_target "All targets" "4"

echo

# Check for control file updates
echo "Control System:"
if [ -f "configs/control/optimization_mode.yaml" ]; then
    last_updated=$(grep "last_updated:" configs/control/optimization_mode.yaml | cut -d'"' -f2)
    echo -e "Control file last updated: ${GREEN}$last_updated${NC}"
else
    echo -e "${RED}✗ Control file not found${NC}"
fi

echo
echo "=== Health Check Complete ==="