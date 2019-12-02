package october

import (
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"strconv"

	"go.uber.org/zap"
)

/*var (
	octoberMetricNS     = metrics.NewNamespace("october")
	octoberHealthSubsys = octoberMetricNS.WithSubsystem("health")

	healthCounterRequests = octoberHealthSubsys.NewCounter(metrics.Opts{
		Name: "endpoint_requests",
		Help: "Total number of October health check endpoint requests",
	})

	healthCounterResponses = octoberHealthSubsys.NewCounterVec(
		metrics.Opts{
			Name: "endpoint_responses",
			Help: "Total number of October health check endpoint responses served, by response code",
		},
		[]string{"code", "health_status"},
	)

	healthLatencySummary = octoberHealthSubsys.NewSummary(metrics.SummaryOpts{
		Name:       "latency",
		Help:       "Summary of October health check endpoint latency",
		Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001, 0.999: 0.0001},
	})
)*/

func init() {

	//octoberMetricNS.MustRegister()
	//octoberHealthSubsys.MustRegister()
	//prometheus.MustRegister(healthCounterRequests)
	//prometheus.MustRegister(healthCounterResponses)
	//prometheus.MustRegister(lastHealthEndpointLatency)
}

func WithHealthCheck(o *OctoberServer, name string, hc HealthCheck) *OctoberServer {
	o.healthChecks.AddCheck(name, hc)
	return o
}

func NewOctoberServer(mode Mode, port int) *OctoberServer {

	if port == 0 {
		port = 10010
	}

	// /healthChecks.AddCheck("october", check)

	zap.S().Named("OCTOBER").Infof("Configuring server with mode %s", mode)

	return &OctoberServer{
		server:       &http.Server{},
		mode:         mode,
		healthChecks: make(HealthChecks),
		checkLock:    &sync.Mutex{},

		octoberBindAddress: "0.0.0.0",
		octoberBindPort:    port,
	}
}

type OctoberServer struct {
	server                     *http.Server
	mode                       Mode
	healthChecks               HealthChecks
	checkLock                  *sync.Mutex // We only want 1 check to go on at once
	lastHealthResult           HealthCheckResult
	lastHealthResultBackground bool

	backgroundCheckHistory []HealthCheckResult
	endpointCheckHistory   []HealthCheckResult

	octoberBindAddress string
	octoberBindPort    int
}

func (o *OctoberServer) buildServerMux() *http.ServeMux {
	mux := http.NewServeMux()

	// Serve prometheus metrics
	mux.Handle("/metrics", promhttp.Handler())
	//mux.Handle("/influxdb", metrichttp.InfluxDBHandler())
	mux.HandleFunc("/health", healthHTTPHandler(o.healthChecks))

	if o.mode != PROD {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return mux
}


func (o *OctoberServer) GenerateGQLGenServerServerFromEnv() (*GQLGenServer, error) {

	zap.L().Named("OCTOBER").Info("Generating controlled gqlgen server from environment variables")

	address := "0.0.0.0"
	port := 8080

	envPort := strings.TrimSpace(os.Getenv(gqlPortEnvVariable))
	if envPort != "" {
		var err error
		port, err = strconv.Atoi(envPort)
		if err != nil {
			return nil, err
		}
	}


	server := &GQLGenServer{
		mode:   o.mode,

		healthChecks: o.healthChecks,
		address: address,
		port:    port,
	}

	return server, nil

}

func (o *OctoberServer) MustGenerateGQLGenServerServerFromEnv() *GQLGenServer {

	server, err := o.GenerateGQLGenServerServerFromEnv()

	if err != nil {
		zap.L().Named("OCTOBER").Fatal("Failed to generate controlled gqlgen server from environment variables", zap.Error(err))
	}

	return server
}

/*func (o *OctoberServer) Start(grpcServer *GRPCServer, gracefulSignals ...os.Signal) {

	// Initialize stop coordinators
	// Used to coordinate stopping the October server and the controller GRPC server (if provided)
	stopChan := make(chan struct{})
	stopping := false
	stoppingMu := &sync.Mutex{}

	// Start by checking if we have graceful signals to handle
	// We want to be sure this is started before anything else
	if len(gracefulSignals) > 0 {

		var signalString string

		for _, gs := range gracefulSignals {
			signalString += fmt.Sprintf("%s, ", gs)
		}

		signalString = strings.TrimSuffix(signalString, ", ")

		zap.S().Named("OCTOBER").Infof("Starting graceful shutdown handler for signals: %s", signalString)
		OnSignal(func(sig os.Signal) {

			stoppingMu.Lock()

			if stopping == false {
				stopping = true
				zap.S().Named("OCTOBER").Info("Starting October graceful shutdown from graceful signal")
				stopChan <- struct{}{}
			} else {
				zap.S().Named("OCTOBER").Info("Ignored graceful shutdown signal, graceful shutdown already initiated")
			}

			stoppingMu.Unlock()

		}, gracefulSignals...)

	}

	address := fmt.Sprintf("%s:%d", o.octoberBindAddress, o.octoberBindPort)
	zap.S().Named("OCTOBER").Infof("Starting server (%s)...", address)
	lis, err := net.Listen("tcp", address)

	if lis != nil {
		defer lis.Close()
	}

	if err != nil {
		zap.L().Named("OCTOBER").Fatal(fmt.Sprintf("October server failed to listen at %s", address), zap.Error(err))
	}

	o.server.Handler = o.buildServerMux()

	// Channel to use to allow coordination between when we attempt to shutdown the October server and when it actually shuts down
	octoberStoppedChan := make(chan struct{})
	// Channel to use to allow coordination between when we attempt to shutdown the GRPC server and when it actually shuts down
	grpcStoppedChan := make(chan struct{})
	// We want to start our grpc server (if present) BEFORE health checks
	// This allows the server to be alive successfully before health checks are enabled
	if grpcServer != nil {

		go func() {

			serveErr := grpcServer.Start()

			if serveErr != nil {
				zap.S().Named("OCTOBER").Error("Controlled GRPC server died with error", zap.Error(serveErr))
			} else {
				zap.S().Named("OCTOBER").Info("Controlled GRPC server closed")
			}

			close(grpcStoppedChan)

			stoppingMu.Lock()
			if stopping == false {
				stopping = true
				if serveErr != nil {
					zap.S().Named("OCTOBER").Info("Starting October graceful shutdown from controlled GRPC server error death", zap.Error(serveErr))
				} else {
					zap.S().Named("OCTOBER").Info("Starting October graceful shutdown from controlled GRPC server death")
				}

				stopChan <- struct{}{}
			}
			stoppingMu.Unlock()

		}()
	}

	go func() {

		serveErr := o.server.Serve(lis)

		if serveErr != nil {
			zap.S().Named("OCTOBER").Error("October server died with error", zap.Error(serveErr))
		} else {
			zap.S().Named("OCTOBER").Info("October server closed")

		}

		close(octoberStoppedChan)

		stoppingMu.Lock()
		if stopping == false {
			stopping = true
			if serveErr != nil {
				zap.S().Named("OCTOBER").Info("Starting October graceful shutdown from october server error death", zap.Error(serveErr))
			} else {
				zap.S().Named("OCTOBER").Info("Starting October graceful shutdown from october server death")
			}

			stopChan <- struct{}{}
		}
		stoppingMu.Unlock()

	}()

	var closeWg sync.WaitGroup
	done := make(chan struct{})

	select {
	case <-stopChan:
		if grpcServer != nil {
			closeWg.Add(1)
			go func() {
				defer closeWg.Done()
				grpcServer.Shutdown(context.Background())
				select {
				case <-grpcStoppedChan:
					zap.S().Named("OCTOBER").Info("Controlled GRPC server shutdown complete")
					return
				}

			}()
		}

		closeWg.Add(1)
		go func() {
			defer closeWg.Done()
			o.Shutdown(context.Background())
			select {
			case <-octoberStoppedChan:
				zap.S().Named("OCTOBER").Info("October server shutdown complete")
				return
			}

		}()

	}

	go func() {
		closeWg.Wait()
		close(done)
	}()

	select {
	case <-done:
		zap.S().Named("OCTOBER").Info("Stopped after server shutdown sequences completed")
	}

}

func (o *OctoberServer) Shutdown(ctx context.Context) error {

	address := fmt.Sprintf("%s:%d", o.octoberBindAddress, o.octoberBindPort)
	zap.S().Named("OCTOBER").Infof("Gracefully stopping server (%s)...", address)

	return o.server.Shutdown(ctx)

}*/
