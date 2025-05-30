package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

var (
	processExecNames = []string{"java_critical_payments", "java_critical_orders", "java_app_frontend", "java_app_backend", "python_api_worker", "python_data_processor", "node_gateway", "nginx_ingress", "postgres_primary", "custom_app_alpha", "custom_app_beta", "sidecar_envoy_proxy", "data_pipeline_job", "cache_redis_server", "log_aggregator_fluentbit", "stress-ng"}
	processOwners    = []string{"payments_user", "orders_user", "app_user", "api_user", "system_user", "data_user", "infra_user", "phoenix_bench_user"}
	baseHostnames    = []string{"web", "app", "db", "cache", "worker", "stream", "loadgen-k8s"}
	containerIDs     = make([]string, 150)
	k8sNamespaces    = []string{"prod-critical", "prod-apps", "staging-apps", "dev-team-a", "infra-services", "default-ns"}
	k8sPodNamePrefix = []string{"payments", "orders", "frontend", "backend", "api", "datajob", "cache-node", "logging-agg", "monitoring-agent", "job-runner"}
	k8sNodeSuffix    = []string{"az1-node", "az2-node", "az3-node"}
)

type processState struct {
	otelResource            *resource.Resource
	metricAttrs             attribute.Set
	pid                     int
	execName                string
	owner                   string
	cmdLine                 string
	containerID             string
	memUsageBytes           float64
	cpuTimeTotal            float64
	threadCount             float64
	openFDCount             float64
	diskReadBytes           float64
	diskWriteBytes          float64
	isHeavyHitter           bool
	memLeakRateBytesPerTick float64
	fdLeakRatePerTick       float64
}

var (
	activeProcesses         map[string][]*processState // Keyed by hostname
	activeProcessesMutex    sync.RWMutex
	processCPUCounter       metric.Float64Counter
	processDiskReadCounter  metric.Float64Counter
	processDiskWriteCounter metric.Float64Counter
)

func initSeedData() {
	rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < len(containerIDs); i++ {
		containerIDs[i] = fmt.Sprintf("cid-%04d-%x%x", i, rand.Int63n(0xFFFFFF), rand.Int63n(0xFFFFFF))
	}
}

func createOtelResourceForProcess(hostname, k8sNamespace, k8sPodName, k8sNodeName, containerName string) *resource.Resource {
	return resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.HostNameKey.String(hostname),
		semconv.ServiceNameKey.String(strings.Split(k8sPodName, "-")[0]), // service from pod prefix
		semconv.ServiceInstanceIDKey.String(k8sPodName),
		attribute.String("instrumentation.provider", "synthetic-generator-v3-gu"),
		attribute.String("benchmark.id", os.Getenv("BENCHMARK_ID")),
		attribute.String("deployment.environment", os.Getenv("DEPLOYMENT_ENV")),
		semconv.K8SNamespaceNameKey.String(k8sNamespace),
		semconv.K8SPodNameKey.String(k8sPodName),
		semconv.K8SNodeNameKey.String(k8sNodeName),
		semconv.K8SContainerNameKey.String(containerName),
	)
}

func initMeterProvider(ctx context.Context) (*sdkmetric.MeterProvider, error) {
	otlpEndpointWithScheme := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if otlpEndpointWithScheme == "" {
		log.Println("WARN (Generator): OTEL_EXPORTER_OTLP_ENDPOINT not set. Metrics will not be exported via OTLP from generator.")
		return sdkmetric.NewMeterProvider(sdkmetric.WithResource(resource.Default())), nil
	}

	endpointParts := strings.SplitN(otlpEndpointWithScheme, "://", 2)
	var exporterEndpoint string
	if len(endpointParts) == 2 {
		exporterEndpoint = endpointParts[1]
	} else {
		exporterEndpoint = endpointParts[0]
	}

	log.Printf("INFO (Generator): OTLP Exporter targeting: %s (from OTEL_EXPORTER_OTLP_ENDPOINT: %s)", exporterEndpoint, otlpEndpointWithScheme)

	// Add retry configuration with backoff
	var exporter *otlpmetrichttp.Exporter
	maxRetries := 5

	for i := 0; i < maxRetries; i++ {
		var err error
		exporter, err = otlpmetrichttp.New(ctx,
			otlpmetrichttp.WithEndpoint(exporterEndpoint),
			otlpmetrichttp.WithInsecure(),
			otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression),
			otlpmetrichttp.WithTimeout(15*time.Second),
		)

		if err == nil {
			break // Successfully created exporter
		}

		if i == maxRetries-1 {
			return nil, fmt.Errorf("synthetic-generator: failed to create OTLP metric exporter after %d attempts: %w", maxRetries, err)
		}

		retryDelay := time.Duration(1<<uint(i)) * time.Second // Exponential backoff
		log.Printf("WARN (Generator): Failed to create OTLP exporter (attempt %d/%d): %v. Retrying in %v...",
			i+1, maxRetries, err, retryDelay)
		time.Sleep(retryDelay)
	}

	// Get resource configuration with fallback mechanisms
	res, err := createResource()
	if err != nil {
		log.Printf("WARN (Generator): Failed to create resource: %v. Using default resource.", err)
		res = resource.Default()
	}

	// Set up the meter provider with memory management constraints
	memLimit := getMemoryLimit()
	log.Printf("INFO (Generator): Configuring with memory limit: %d bytes", memLimit)

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithReader(sdkmetric.NewPeriodicReader(exporter,
			sdkmetric.WithInterval(10*time.Second),
			sdkmetric.WithTimeout(30*time.Second),
		)),
		sdkmetric.WithResource(res),
		// Set reasonable memory limits for collection
		sdkmetric.WithView(sdkmetric.NewView(
			sdkmetric.Instrument{Scope: instrumentation.Scope{Name: "*"}},
			sdkmetric.Stream{AggregationTemporality: sdkmetric.CumulativeTemporality},
		)),
	)
	return mp, nil
}

// createResource creates an OpenTelemetry resource with proper error handling
func createResource() (*resource.Resource, error) {
	// Create a base resource from environment variables
	envRes, err := resource.New(context.Background(),
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource from environment: %w", err)
	}

	// Add custom attributes
	customRes, err := resource.Merge(
		envRes,
		resource.NewWithAttributes(
			semconv.SchemaURL,
			attribute.String("instrumentation.provider", "synthetic-generator-v3-gu"),
			attribute.String("benchmark.id", os.Getenv("BENCHMARK_ID")),
			attribute.String("deployment.environment", os.Getenv("DEPLOYMENT_ENV")),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to merge resources: %w", err)
	}

	return customRes, nil
}

// getMemoryLimit determines reasonable memory limits based on container constraints
func getMemoryLimit() int64 {
	// Default memory limit (500 MB)
	defaultLimit := int64(500 * 1024 * 1024)

	// Try to read cgroup memory limit if available
	content, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes")
	if err != nil {
		return defaultLimit
	}

	limitStr := strings.TrimSpace(string(content))
	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil {
		return defaultLimit
	}

	// If limit is unreasonably high (or unlimited), use default
	if limit > 8*1024*1024*1024 || limit <= 0 {
		return defaultLimit
	}

	// Use 80% of the container memory limit
	return int64(float64(limit) * 0.8)
}

func generateProcessMetricAttributes(p *processState) attribute.Set {
	attrs := []attribute.KeyValue{
		semconv.ProcessExecutableNameKey.String(p.execName),
		semconv.ProcessOwnerKey.String(p.owner),
		semconv.ProcessPIDKey.Int(p.pid),
		semconv.ProcessCommandLineKey.String(p.cmdLine),
	}
	if p.containerID != "" {
		attrs = append(attrs, semconv.ContainerIDKey.String(p.containerID))
	}
	tier := "tier3_support_generic"
	if strings.Contains(p.execName, "critical") {
		tier = "tier1_critical_core"
	} else if strings.HasPrefix(p.execName, "java_app") || strings.HasPrefix(p.execName, "python_api") || strings.HasPrefix(p.execName, "node_gateway") {
		tier = "tier2_application_main"
	} else if strings.Contains(p.execName, "nginx") || strings.Contains(p.execName, "postgres") {
		tier = "tier2_infra_support"
	}
	attrs = append(attrs, attribute.String("custom.service.tier_simulated", tier))
	if p.isHeavyHitter {
		attrs = append(attrs, attribute.Bool("custom.process.is_heavy_hitter_simulated", true))
	}
	return attribute.NewSet(attrs...)
}

func observeProcessMemory(_ context.Context, observer metric.Float64Observer) error {
	activeProcessesMutex.RLock()
	defer activeProcessesMutex.RUnlock()
	for _, hostProcs := range activeProcesses {
		for _, proc := range hostProcs {
			observer.Observe(proc.memUsageBytes, metric.WithAttributeSet(proc.metricAttrs))
		}
	}
	return nil
}

func observeProcessThreads(_ context.Context, observer metric.Float64Observer) error {
	activeProcessesMutex.RLock()
	defer activeProcessesMutex.RUnlock()
	for _, hostProcs := range activeProcesses {
		for _, proc := range hostProcs {
			observer.Observe(proc.threadCount, metric.WithAttributeSet(proc.metricAttrs))
		}
	}
	return nil
}

func observeProcessFDs(_ context.Context, observer metric.Float64Observer) error {
	activeProcessesMutex.RLock()
	defer activeProcessesMutex.RUnlock()
	for _, hostProcs := range activeProcesses {
		for _, proc := range hostProcs {
			observer.Observe(proc.openFDCount, metric.WithAttributeSet(proc.metricAttrs))
		}
	}
	return nil
}

func cleanupResources(ctx context.Context, mp *sdkmetric.MeterProvider) {
	log.Println("INFO (Generator): Shutting down and cleaning up resources...")

	// Clear process maps to free memory
	activeProcessesMutex.Lock()
	for hostname := range activeProcesses {
		activeProcesses[hostname] = nil
	}
	activeProcesses = nil
	activeProcessesMutex.Unlock()

	// Shutdown meter provider gracefully
	if mp != nil {
		ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		if err := mp.Shutdown(ctx); err != nil {
			log.Printf("ERROR (Generator): Failed to shutdown meter provider: %v", err)
		}
	}

	log.Println("INFO (Generator): Cleanup completed")
}

func setupGracefulShutdown(ctx context.Context, cancel context.CancelFunc, mp *sdkmetric.MeterProvider) {
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigs
		log.Printf("INFO (Generator): Received signal %v, initiating graceful shutdown", sig)
		cancel()
		cleanupResources(context.Background(), mp)
	}()
}

// monitorResourceUsage periodically checks and logs resource usage
func monitorResourceUsage(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)

			log.Printf("INFO (Generator): Resource usage - Goroutines: %d, Alloc: %.2f MB, Sys: %.2f MB",
				runtime.NumGoroutine(),
				float64(mem.Alloc)/1024/1024,
				float64(mem.Sys)/1024/1024)

			// Check if active processes map is growing too large
			activeProcessesMutex.RLock()
			totalActiveProcs := 0
			for _, procs := range activeProcesses {
				totalActiveProcs += len(procs)
			}
			activeProcessesMutex.RUnlock()

			log.Printf("INFO (Generator): Active processes count: %d", totalActiveProcs)

			// Force garbage collection if memory usage is high
			if mem.Alloc > 400*1024*1024 { // 400 MB
				log.Println("INFO (Generator): High memory usage detected, running garbage collection")
				runtime.GC()
			}
		}
	}
}

func main() {
	// Create a cancellable context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Initialize random seed data
	initSeedData()
	
	log.Println("INFO (Generator): Phoenix vNext Synthetic Generator starting up...")

	// Load and validate configuration from environment variables with defaults
	processCountPerHostStr := os.Getenv("SYNTHETIC_PROCESS_COUNT_PER_HOST")
	processCountPerHost, err := strconv.Atoi(processCountPerHostStr)
	if err != nil || processCountPerHost <= 0 {
		if processCountPerHostStr != "" {
			log.Printf("WARN (Generator): Invalid SYNTHETIC_PROCESS_COUNT_PER_HOST value '%s', using default: 150", processCountPerHostStr)
		}
		processCountPerHost = 150
	}

	hostCountStr := os.Getenv("SYNTHETIC_HOST_COUNT")
	hostCount, err := strconv.Atoi(hostCountStr)
	if err != nil || hostCount <= 0 {
		if hostCountStr != "" {
			log.Printf("WARN (Generator): Invalid SYNTHETIC_HOST_COUNT value '%s', using default: 3", hostCountStr)
		}
		hostCount = 3
	}

	metricRateSStr := os.Getenv("SYNTHETIC_METRIC_EMIT_INTERVAL_S")
	metricRateS, err := strconv.Atoi(metricRateSStr)
	if err != nil || metricRateS <= 0 {
		if metricRateSStr != "" {
			log.Printf("WARN (Generator): Invalid SYNTHETIC_METRIC_EMIT_INTERVAL_S value '%s', using default: 15", metricRateSStr)
		}
		metricRateS = 15
	}
	
	// Initialize OpenTelemetry meter provider with error handling
	mp, err := initMeterProvider(ctx)
	if err != nil {
		log.Fatalf("ERROR (Generator): Failed to initialize meter provider: %v", err)
	}
	otel.SetMeterProvider(mp)
	
	// Setup graceful shutdown handler
	setupGracefulShutdown(ctx, cancel, mp)
	
	// Start resource usage monitoring in background
	go monitorResourceUsage(ctx, 60*time.Second)

	meter := otel.Meter("phoenix.v3.ultimate.synthetic.generator")
	// Create metrics with proper error handling
	createMetric := func(name, description, unit string) (metric.Float64Counter, error) {
		counter, err := meter.Float64Counter(
			name,
			metric.WithDescription(description),
			metric.WithUnit(unit),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create %s counter: %w", name, err)
		}
		return counter, nil
	}
	
	// Create all required metrics
	var instErr error
	processCPUCounter, instErr = createMetric(
		"process.cpu.time",
		"Cumulative CPU time consumed by the process, reported as delta",
		"s")
	if instErr != nil {
		log.Fatalf("ERROR (Generator): %v", instErr)
	}
	
	processDiskReadCounter, instErr = createMetric(
		"process.disk.io.read_bytes",
		"Cumulative disk read bytes, reported as delta",
		"By")
	if instErr != nil {
		log.Fatalf("ERROR (Generator): %v", instErr)
	}
	
	processDiskWriteCounter, instErr = createMetric(
		"process.disk.io.write_bytes",
		"Cumulative disk write bytes, reported as delta",
		"By")
	if instErr != nil {
		log.Fatalf("ERROR (Generator): %v", instErr)
	}

	activeProcesses = make(map[string][]*processState)
	totalProcessesGenerated := 0

	log.Printf("INFO (Generator): Initializing %d hosts, each with %d processes...", hostCount, processCountPerHost)
	for h := 0; h < hostCount; h++ {
		k8sNodeName := fmt.Sprintf("%s-%s", baseHostnames[h%len(baseHostnames)], k8sNodeSuffix[h%len(k8sNodeSuffix)])
		hostnameForProcs := k8sNodeName

		activeProcesses[hostnameForProcs] = []*processState{}
		k8sNs := k8sNamespaces[rand.Intn(len(k8sNamespaces))]

		for i := 0; i < processCountPerHost; i++ {
			totalProcessesGenerated++
			pid := 1000 + totalProcessesGenerated
			execName := processExecNames[rand.Intn(len(processExecNames))]
			owner := processOwners[rand.Intn(len(processOwners))]
			containerIDVal := ""
			if rand.Float32() < 0.7 {
				containerIDVal = containerIDs[rand.Intn(len(containerIDs))]
			}

			podNameBase := strings.ReplaceAll(strings.Split(execName, "_")[0], "-", "")
			if len(podNameBase) > 12 {
				podNameBase = podNameBase[:12]
			}
			k8sPod := fmt.Sprintf("%s-%s-%x", k8sPodNamePrefix[rand.Intn(len(k8sPodNamePrefix))], podNameBase, rand.Intn(0xfff))
			containerName := execName

			cmdLine := fmt.Sprintf("/opt/app/%s --config /etc/app/config.yaml --instance %d --pod %s --namespace %s", execName, i%20, k8sPod, k8sNs)
			if strings.Contains(execName, "java") {
				heapSize := 128 + rand.Intn(8)*32
				appNameForCmd := strings.ReplaceAll(strings.ReplaceAll(execName, "java_", ""), "_", "-")
				cmdLine = fmt.Sprintf("/usr/bin/java -Dapp.name=%s -Dspring.profiles.active=%s -Xms%dm -Xmx%dm -jar /opt/apps/%s.jar --server.port=%d", appNameForCmd, k8sNs, heapSize/2, heapSize, appNameForCmd, 8000+i%100)
			}

			isHeavy := rand.Float32() < 0.08

			ps := &processState{
				otelResource:            createOtelResourceForProcess(hostnameForProcs, k8sNs, k8sPod, k8sNodeName, containerName),
				pid:                     pid,
				execName:                execName,
				owner:                   owner,
				cmdLine:                 cmdLine,
				containerID:             containerIDVal,
				memUsageBytes:           rand.Float64() * float64(64+rand.Intn(1024)) * 1024 * 1024,
				cpuTimeTotal:            rand.Float64() * float64(100+rand.Intn(3900)),
				threadCount:             float64(5 + rand.Intn(80)),
				openFDCount:             float64(10 + rand.Intn(300)),
				diskReadBytes:           rand.Float64() * 1024 * 1024 * float64(20+rand.Intn(180)),
				diskWriteBytes:          rand.Float64() * 1024 * 1024 * float64(10+rand.Intn(90)),
				isHeavyHitter:           isHeavy,
				memLeakRateBytesPerTick: 0,
				fdLeakRatePerTick:       0,
			}
			if rand.Float32() < 0.02 {
				ps.memLeakRateBytesPerTick = rand.Float64() * 5 * 1024 * 1024
				// log.Printf("INFO (Generator): Simulating memory leak for %s (PID %d) on %s at %.2f MB/tick", ps.execName, ps.pid, hostnameForProcs, ps.memLeakRateBytesPerTick/(1024*1024))
			}
			if rand.Float32() < 0.01 {
				ps.fdLeakRatePerTick = rand.Float64() * 3
				// log.Printf("INFO (Generator): Simulating FD leak for %s (PID %d) on %s at %.0f FDs/tick", ps.execName, ps.pid, hostnameForProcs, ps.fdLeakRatePerTick)
			}

			ps.metricAttrs = generateProcessMetricAttributes(ps)
			activeProcesses[hostnameForProcs] = append(activeProcesses[hostnameForProcs], ps)
		}
	}

	_, instErr = meter.Float64ObservableGauge("process.memory.usage",
		metric.WithDescription("Resident Set Size of the process"), metric.WithUnit("By"),
		metric.WithFloat64Callback(observeProcessMemory))
	if instErr != nil {
		log.Fatalf("Failed to create process.memory.usage gauge: %v", instErr)
	}

	_, instErr = meter.Float64ObservableGauge("process.threads",
		metric.WithDescription("Number of threads in the process"), metric.WithUnit("{threads}"),
		metric.WithFloat64Callback(observeProcessThreads))
	if instErr != nil {
		log.Fatalf("Failed to create process.threads gauge: %v", instErr)
	}

	_, instErr = meter.Float64ObservableGauge("process.open_file_descriptors",
		metric.WithDescription("Number of open file descriptors"), metric.WithUnit("{descriptors}"),
		metric.WithFloat64Callback(observeProcessFDs))
	if instErr != nil {
		log.Fatalf("Failed to create process.open_file_descriptors gauge: %v", instErr)
	}

	log.Printf("INFO (Generator): Initialized %d hosts, %d total processes. Starting metric emission every %d seconds...", hostCount, totalProcessesGenerated, metricRateS)

	ticker := time.NewTicker(time.Duration(metricRateS) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			activeProcessesMutex.Lock()
			var totalMetricPointsEmittedThisTick int64
			for _, hostProcs := range activeProcesses {
				for i := range hostProcs {
					proc := hostProcs[i]

					cpuDelta := rand.Float64()*0.7 + 0.001
					if proc.isHeavyHitter || strings.Contains(proc.execName, "critical") {
						cpuDelta *= (2.0 + rand.Float64()*3.0)
					}
					if strings.HasPrefix(proc.execName, "sidecar") {
						cpuDelta *= 0.15
					}
					proc.cpuTimeTotal += cpuDelta
					processCPUCounter.Add(ctx, cpuDelta, metric.WithAttributeSet(proc.metricAttrs))
					totalMetricPointsEmittedThisTick++

					readDelta := rand.Float64() * 1024 * float64(5+rand.Intn(150))
					writeDelta := rand.Float64() * 1024 * float64(2+rand.Intn(75))
					if proc.isHeavyHitter || strings.Contains(proc.execName, "postgres") || strings.Contains(proc.execName, "data_pipeline") {
						readDelta *= 3
						writeDelta *= 3
					}
					proc.diskReadBytes += readDelta
					proc.diskWriteBytes += writeDelta
					processDiskReadCounter.Add(ctx, readDelta, metric.WithAttributeSet(proc.metricAttrs))
					processDiskWriteCounter.Add(ctx, writeDelta, metric.WithAttributeSet(proc.metricAttrs))
					totalMetricPointsEmittedThisTick += 2

					memChange := (rand.Float64() - 0.49) * float64(10+rand.Intn(30)) * 1024 * 1024
					if proc.isHeavyHitter {
						memChange *= 1.2
					}
					proc.memUsageBytes += memChange + proc.memLeakRateBytesPerTick
					if proc.memUsageBytes < (10 * 1024 * 1024) {
						proc.memUsageBytes = 10 * 1024 * 1024
					}
					if proc.memUsageBytes > (1800 * 1024 * 1024) {
						proc.memUsageBytes = 1800 * 1024 * 1024
					} // Max ~1.8GB

					proc.threadCount += (rand.Float64() - 0.48) * 4
					if proc.threadCount < 2 {
						proc.threadCount = 2
					}
					if proc.threadCount > 200 {
						proc.threadCount = 200
					}
					if proc.isHeavyHitter {
						proc.threadCount += float64(rand.Intn(8))
					}

					proc.openFDCount += (rand.Float64()-0.47)*10 + proc.fdLeakRatePerTick
					if proc.openFDCount < 5 {
						proc.openFDCount = 5
					}
					if proc.openFDCount > 900 {
						proc.openFDCount = 900
					}

					if rand.Float32() < 0.0005 {
						_ = proc.pid
						_ = proc.execName
						proc.pid = 70000 + rand.Intn(30000)
						if rand.Float32() < 0.05 {
							baseName := strings.Split(proc.execName, "_v")[0]
							baseName = strings.Split(baseName, "_restarted")[0]
							proc.execName = fmt.Sprintf("%s_restarted_v%.1f", baseName, (rand.Float32()*2)+1.0)
						}
						proc.cmdLine = fmt.Sprintf("/opt/bin/%s --reconfig --new-instance-%d", proc.execName, proc.pid)
						proc.metricAttrs = generateProcessMetricAttributes(proc)
						proc.cpuTimeTotal = rand.Float64() * 100.0
						proc.memUsageBytes = rand.Float64() * float64(64+rand.Intn(256)) * 1024 * 1024
						proc.threadCount = float64(5 + rand.Intn(20))
						proc.openFDCount = float64(10 + rand.Intn(50))
						proc.isHeavyHitter = rand.Float32() < 0.08
						proc.memLeakRateBytesPerTick = 0
						proc.fdLeakRatePerTick = 0
						if rand.Float32() < 0.02 {
							proc.memLeakRateBytesPerTick = rand.Float64() * 2 * 1024 * 1024
						}
						// log.Printf("INFO (Generator): Host %s: Process %s (Old PID %d) 'restarted' as %s PID %d", hostname, oldExecName, oldPID, proc.execName, proc.pid)
					}
				}
			}
			activeProcessesMutex.Unlock()
			log.Printf("INFO (Generator): Tick completed. Emitted approx %d counter data points. Gauge values updated.", totalMetricPointsEmittedThisTick)
		case <-ctx.Done():
			log.Println("INFO (Generator): Shutdown signal received.")
			return
		}
	}
}
