// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"net"
	"net/http"
	"runtime"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/tomb.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/introspection/pprof"
)

var logger = loggo.GetLogger("juju.worker.introspection")

// DepEngineReporter provides insight into the running dependency engine of the agent.
type DepEngineReporter interface {
	// Report returns a map describing the state of the receiver. It is expected
	// to be goroutine-safe.
	Report() map[string]interface{}
}

// IntrospectionReporter provides a simple method that the introspection
// worker will output for the entity.
type IntrospectionReporter interface {
	IntrospectionReport() string
}

// Config describes the arguments required to create the introspection worker.
type Config struct {
	SocketName         string
	DepEngine          DepEngineReporter
	StatePool          IntrospectionReporter
	PrometheusGatherer prometheus.Gatherer
}

// Validate checks the config values to assert they are valid to create the worker.
func (c *Config) Validate() error {
	if c.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if c.PrometheusGatherer == nil {
		return errors.NotValidf("nil PrometheusGatherer")
	}
	return nil
}

// socketListener is a worker and constructed with NewWorker.
type socketListener struct {
	tomb               tomb.Tomb
	listener           *net.UnixListener
	depEngine          DepEngineReporter
	statePool          IntrospectionReporter
	prometheusGatherer prometheus.Gatherer
	done               chan struct{}
}

// NewWorker starts an http server listening on an abstract domain socket
// which will be created with the specified name.
func NewWorker(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	if runtime.GOOS != "linux" {
		return nil, errors.NotSupportedf("os %q", runtime.GOOS)
	}

	path := "@" + config.SocketName
	addr, err := net.ResolveUnixAddr("unix", path)
	if err != nil {
		return nil, errors.Annotate(err, "unable to resolve unix socket")
	}

	l, err := net.ListenUnix("unix", addr)
	if err != nil {
		return nil, errors.Annotate(err, "unable to listen on unix socket")
	}
	logger.Debugf("introspection worker listening on %q", path)

	w := &socketListener{
		listener:           l,
		depEngine:          config.DepEngine,
		statePool:          config.StatePool,
		prometheusGatherer: config.PrometheusGatherer,
		done:               make(chan struct{}),
	}
	go w.serve()
	go w.run()
	return w, nil
}

func (w *socketListener) serve() {
	mux := http.NewServeMux()
	RegisterHTTPHandlers(
		ReportSources{
			DependencyEngine:   w.depEngine,
			StatePool:          w.statePool,
			PrometheusGatherer: w.prometheusGatherer,
		}, mux.Handle)

	srv := http.Server{Handler: mux}
	logger.Debugf("stats worker now serving")
	defer logger.Debugf("stats worker serving finished")
	defer close(w.done)
	srv.Serve(w.listener)
}

func (w *socketListener) run() {
	defer w.tomb.Done()
	defer logger.Debugf("stats worker finished")
	<-w.tomb.Dying()
	logger.Debugf("stats worker closing listener")
	w.listener.Close()
	// Don't mark the worker as done until the serve goroutine has finished.
	<-w.done
}

// Kill implements worker.Worker.
func (w *socketListener) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *socketListener) Wait() error {
	return w.tomb.Wait()
}

// ReportSources are the various information sources that are exposed
// through the introspection facility.
type ReportSources struct {
	DependencyEngine   DepEngineReporter
	StatePool          IntrospectionReporter
	PrometheusGatherer prometheus.Gatherer
}

// AddHandlers calls the given function with http.Handlers
// that serve agent introspection requests. The function will
// be called with a path; the function may alter the path
// as it sees fit.
func RegisterHTTPHandlers(
	sources ReportSources,
	handle func(path string, h http.Handler),
) {
	handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	handle("/depengine/", depengineHandler{sources.DependencyEngine})
	handle("/statepool/", introspectionReporterHandler{
		name:     "State Pool Report",
		reporter: sources.StatePool,
	})
	handle("/metrics", promhttp.HandlerFor(sources.PrometheusGatherer, promhttp.HandlerOpts{}))
}

type depengineHandler struct {
	reporter DepEngineReporter
}

// ServeHTTP is part of the http.Handler interface.
func (h depengineHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.reporter == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "missing dependency engine reporter")
		return
	}
	bytes, err := yaml.Marshal(h.reporter.Report())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	fmt.Fprint(w, "Dependency Engine Report\n\n")
	w.Write(bytes)
}

type introspectionReporterHandler struct {
	name     string
	reporter IntrospectionReporter
}

// ServeHTTP is part of the http.Handler interface.
func (h introspectionReporterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.reporter == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "%s: missing reporter\n", h.name)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	fmt.Fprintf(w, "%s:\n\n", h.name)
	fmt.Fprint(w, h.reporter.IntrospectionReport())
}
