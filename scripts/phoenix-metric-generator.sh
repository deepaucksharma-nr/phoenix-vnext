#!/bin/bash
# Phoenix v3 - Metric Generator for Complex Pipeline Metrics
# This script generates synthetic metrics that can't be easily created via OTel transforms

set -euo pipefail

# Configuration
PROMETHEUS_PUSHGATEWAY="${PROMETHEUS_PUSHGATEWAY:-http://localhost:9091}"
JOB_NAME="phoenix_metric_generator"
INSTANCE="${HOSTNAME:-localhost}"

# Function to push metrics to Prometheus
push_metric() {
    local metric_name=$1
    local metric_value=$2
    local labels=$3
    local metric_type=${4:-gauge}
    
    cat <<EOF | curl --data-binary @- "${PROMETHEUS_PUSHGATEWAY}/metrics/job/${JOB_NAME}/instance/${INSTANCE}"
# TYPE ${metric_name} ${metric_type}
${metric_name}${labels} ${metric_value}
EOF
}

# Generate pipeline flow data for Sankey diagram
generate_sankey_flow_data() {
    local timestamp=$(date +%s)
    
    # Simulated flow data based on actual metrics
    push_metric "phoenix_pipeline_flow_sankey" "10000" '{source="Raw Metrics",target="Priority Classification",pipeline="all"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "10000" '{source="Priority Classification",target="Full Pipeline",pipeline="full"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "10000" '{source="Priority Classification",target="Optimised Pipeline",pipeline="optimised"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "10000" '{source="Priority Classification",target="Experimental Pipeline",pipeline="experimental"}' "gauge"
    
    push_metric "phoenix_pipeline_flow_sankey" "9800" '{source="Full Pipeline",target="Exported to NR",pipeline="full"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "200" '{source="Full Pipeline",target="Dropped",pipeline="full"}' "gauge"
    
    push_metric "phoenix_pipeline_flow_sankey" "4000" '{source="Optimised Pipeline",target="Exported to NR",pipeline="optimised"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "6000" '{source="Optimised Pipeline",target="Dropped",pipeline="optimised"}' "gauge"
    
    push_metric "phoenix_pipeline_flow_sankey" "1500" '{source="Experimental Pipeline",target="Exported to NR",pipeline="experimental"}' "gauge"
    push_metric "phoenix_pipeline_flow_sankey" "8500" '{source="Experimental Pipeline",target="Dropped",pipeline="experimental"}' "gauge"
}

# Generate process dependency graph data
generate_dependency_graph() {
    # Node data
    push_metric "phoenix_process_dependency_graph" "85" '{process="nginx",type="node",cpu_usage="15",memory_usage="85",criticality="high"}' "gauge"
    push_metric "phoenix_process_dependency_graph" "92" '{process="java_app",type="node",cpu_usage="45",memory_usage="92",criticality="critical"}' "gauge"
    push_metric "phoenix_process_dependency_graph" "65" '{process="postgres",type="node",cpu_usage="25",memory_usage="65",criticality="high"}' "gauge"
    push_metric "phoenix_process_dependency_graph" "45" '{process="redis",type="node",cpu_usage="5",memory_usage="45",criticality="medium"}' "gauge"
    
    # Edge data
    push_metric "phoenix_process_dependency_edges" "150" '{source="nginx",target="java_app",type="edge",requests_per_sec="150"}' "gauge"
    push_metric "phoenix_process_dependency_edges" "75" '{source="java_app",target="postgres",type="edge",queries_per_sec="75"}' "gauge"
    push_metric "phoenix_process_dependency_edges" "200" '{source="java_app",target="redis",type="edge",ops_per_sec="200"}' "gauge"
}

# Generate optimization surface data for 3D plot
generate_optimization_surface() {
    # Generate a 3D surface for cost-performance-cardinality tradeoffs
    for cost in {100..1000..100}; do
        for performance in {60..100..5}; do
            # Calculate cardinality based on cost and performance
            cardinality=$((10000 - (cost * 5) + (performance * 50)))
            push_metric "phoenix_optimization_surface_data" "${cardinality}" "{cost_dimension=\"${cost}\",performance_dimension=\"${performance}\"}" "gauge"
        done
    done
}

# Generate flame graph data for pipeline processing
generate_flamegraph_data() {
    # Simulated processing time breakdown
    push_metric "phoenix_pipeline_processing_profile" "1000" '{stack="receivers;hostmetrics",pipeline="all"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "500" '{stack="receivers;otlp",pipeline="all"}' "gauge"
    
    push_metric "phoenix_pipeline_processing_profile" "200" '{stack="processors;memory_limiter",pipeline="all"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "300" '{stack="processors;transform;enrichment",pipeline="all"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "400" '{stack="processors;transform;priority_classification",pipeline="all"}' "gauge"
    
    push_metric "phoenix_pipeline_processing_profile" "100" '{stack="processors;filter;optimised_selection",pipeline="optimised"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "150" '{stack="processors;groupby;optimised_rollup",pipeline="optimised"}' "gauge"
    
    push_metric "phoenix_pipeline_processing_profile" "50" '{stack="processors;filter;experimental_aggressive",pipeline="experimental"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "75" '{stack="processors;groupby;experimental_rollup",pipeline="experimental"}' "gauge"
    
    push_metric "phoenix_pipeline_processing_profile" "300" '{stack="exporters;prometheus",pipeline="all"}' "gauge"
    push_metric "phoenix_pipeline_processing_profile" "400" '{stack="exporters;newrelic",pipeline="all"}' "gauge"
}

# Generate cost metrics
generate_cost_metrics() {
    # Current hourly cost calculation (simplified)
    local full_ts=$(curl -s "http://localhost:8888/metrics" | grep -E "phoenix_full_output_ts_active" | awk '{print $2}' || echo "10000")
    local opt_ts=$(curl -s "http://localhost:8889/metrics" | grep -E "phoenix_optimised_output_ts_active" | awk '{print $2}' || echo "4000")
    local exp_ts=$(curl -s "http://localhost:8890/metrics" | grep -E "phoenix_experimental_output_ts_active" | awk '{print $2}' || echo "1500")
    
    # Cost per 1000 time series per hour (example: $0.05)
    local cost_per_1k_ts_hour=0.05
    
    local full_cost=$(echo "scale=2; ${full_ts} * ${cost_per_1k_ts_hour} / 1000" | bc)
    local opt_cost=$(echo "scale=2; ${opt_ts} * ${cost_per_1k_ts_hour} / 1000" | bc)
    local exp_cost=$(echo "scale=2; ${exp_ts} * ${cost_per_1k_ts_hour} / 1000" | bc)
    
    push_metric "phoenix_current_hourly_cost_usd" "${full_cost}" '{pipeline="full_fidelity"}' "gauge"
    push_metric "phoenix_current_hourly_cost_usd" "${opt_cost}" '{pipeline="optimised"}' "gauge"
    push_metric "phoenix_current_hourly_cost_usd" "${exp_cost}" '{pipeline="experimental"}' "gauge"
    
    # Cost reduction ratio
    local reduction_ratio=$(echo "scale=2; 1 - (${opt_cost} / ${full_cost})" | bc)
    push_metric "phoenix:cost_reduction_ratio" "${reduction_ratio}" '{comparison="optimised_vs_full"}' "gauge"
}

# Main execution loop
main() {
    echo "Phoenix Metric Generator starting..."
    
    while true; do
        echo "$(date): Generating metrics..."
        
        generate_sankey_flow_data
        generate_dependency_graph
        generate_optimization_surface
        generate_flamegraph_data
        generate_cost_metrics
        
        echo "$(date): Metrics pushed successfully"
        
        # Sleep for 30 seconds before next iteration
        sleep 30
    done
}

# Run main function
main