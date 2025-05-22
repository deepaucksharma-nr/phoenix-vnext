#!/usr/bin/env bash
# Phoenix-vNext Data Flow Validation Script
# Validates that metrics are flowing through all pipelines correctly

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "=== Phoenix-vNext Data Flow Validation ==="
echo

# Function to query Prometheus and check for metrics
check_metric_presence() {
    local metric_name="$1"
    local description="$2"
    local endpoint="$3"
    
    echo -n "Checking $description... "
    
    # Query the specific endpoint for the metric
    local metric_count=$(curl -s "$endpoint" | grep -c "^$metric_name" || echo "0")
    
    if [ "$metric_count" -gt 0 ]; then
        echo -e "${GREEN}✓ Found $metric_count series${NC}"
        return 0
    else
        echo -e "${RED}✗ No metrics found${NC}"
        return 1
    fi
}

# Function to check Prometheus query results
check_prometheus_query() {
    local query="$1"
    local description="$2"
    
    echo -n "Checking $description... "
    
    local encoded_query=$(printf "%s" "$query" | sed 's/ /%20/g' | sed 's/{/%7B/g' | sed 's/}/%7D/g')
    local result=$(curl -s "http://localhost:9090/api/v1/query?query=$encoded_query" | grep -o '"result":\[.*\]' || echo '"result":[]')
    
    if [[ "$result" != '"result":[]' ]]; then
        local value_count=$(echo "$result" | grep -o '"value":\[' | wc -l)
        echo -e "${GREEN}✓ Found $value_count values${NC}"
        return 0
    else
        echo -e "${RED}✗ No data returned${NC}"
        return 1
    fi
}

echo -e "${BLUE}1. Direct Pipeline Output Validation${NC}"
echo "Checking each pipeline endpoint for metrics..."

# Check Full Fidelity Pipeline
check_metric_presence "phoenix_full_final_output" "Full Fidelity Pipeline" "http://localhost:8888/metrics"

# Check Optimized Pipeline  
check_metric_presence "phoenix_opt_final_output" "Optimized Pipeline" "http://localhost:8889/metrics"

# Check Experimental Pipeline
check_metric_presence "phoenix_exp_final_output" "Experimental Pipeline" "http://localhost:8890/metrics"

# Check Observer KPIs
check_metric_presence "phoenix_observer_kpi_store" "Observer KPI Store" "http://localhost:9888/metrics"

echo

echo -e "${BLUE}2. Prometheus Data Ingestion Validation${NC}"
echo "Checking if Prometheus has scraped metrics from each pipeline..."

# Check if Prometheus has metrics from each job
check_prometheus_query "up{job=\"otelcol-main-telemetry\"}" "Main Collector Scrape Status"
check_prometheus_query "up{job=\"otelcol-main-opt-output\"}" "Optimized Pipeline Scrape Status"
check_prometheus_query "up{job=\"otelcol-main-exp-output\"}" "Experimental Pipeline Scrape Status"
check_prometheus_query "up{job=\"otelcol-observer-metrics\"}" "Observer Scrape Status"

echo

echo -e "${BLUE}3. Process Metrics Validation${NC}"
echo "Checking for actual process metrics data..."

# Check for process CPU metrics
check_prometheus_query "phoenix_opt_final_output_process_cpu_time_total" "Process CPU Time Metrics"

# Check for process memory metrics
check_prometheus_query "phoenix_opt_final_output_process_memory_usage" "Process Memory Usage Metrics"

# Check for active time series count
check_prometheus_query "phoenix_opt_final_output_phoenix_optimised_output_ts_active" "Active Time Series Count"

echo

echo -e "${BLUE}4. Cost Reduction KPI Validation${NC}"
echo "Checking derived KPIs and recording rules..."

# Check cost reduction ratio
check_prometheus_query "phoenix:cost_reduction_ratio" "Cost Reduction Ratio KPI"

# Check pipeline cardinality estimates from observer
check_prometheus_query "phoenix_observer_kpi_store_phoenix_pipeline_output_cardinality_estimate" "Pipeline Cardinality Estimates"

echo

echo -e "${BLUE}5. Control System Validation${NC}"
echo "Checking control system metrics and file updates..."

# Check if control file exists and has recent updates
if [ -f "configs/control/optimization_mode.yaml" ]; then
    echo -n "Control file exists... "
    echo -e "${GREEN}✓${NC}"
    
    # Check last update time
    if grep -q "last_updated:" configs/control/optimization_mode.yaml; then
        last_updated=$(grep "last_updated:" configs/control/optimization_mode.yaml | cut -d'"' -f2)
        echo "Last updated: $last_updated"
        
        # Check if update was recent (within last 10 minutes)
        if command -v date >/dev/null 2>&1; then
            current_time=$(date -u +%s)
            update_time=$(date -d "$last_updated" +%s 2>/dev/null || echo "0")
            age=$((current_time - update_time))
            
            if [ "$age" -lt 600 ]; then  # 10 minutes
                echo -e "${GREEN}✓ Control file recently updated${NC}"
            else
                echo -e "${YELLOW}⚠ Control file may be stale (${age}s old)${NC}"
            fi
        fi
    else
        echo -e "${RED}✗ No last_updated field found${NC}"
    fi
else
    echo -e "${RED}✗ Control file not found${NC}"
fi

echo

echo "=== Data Flow Validation Complete ==="
echo
echo -e "${BLUE}Summary:${NC}"
echo "• Check Grafana dashboards at http://localhost:3000"
echo "• Monitor Prometheus targets at http://localhost:9090/targets"  
echo "• View pipeline metrics directly at collector endpoints"
echo "• Examine control file: configs/control/optimization_mode.yaml"