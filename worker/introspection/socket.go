// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/tomb.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/presence"
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
	PubSub             IntrospectionReporter
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	Presence           presence.Recorder
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
	pubsub             IntrospectionReporter
	machineLock        machinelock.Lock
	prometheusGatherer prometheus.Gatherer
	presence           presence.Recorder
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
		pubsub:             config.PubSub,
		machineLock:        config.MachineLock,
		prometheusGatherer: config.PrometheusGatherer,
		presence:           config.Presence,
		done:               make(chan struct{}),
	}
	go w.serve()
	w.tomb.Go(w.run)
	return w, nil
}

func (w *socketListener) serve() {
	mux := http.NewServeMux()
	RegisterHTTPHandlers(
		ReportSources{
			DependencyEngine:   w.depEngine,
			StatePool:          w.statePool,
			PubSub:             w.pubsub,
			MachineLock:        w.machineLock,
			PrometheusGatherer: w.prometheusGatherer,
			Presence:           w.presence,
		}, mux.Handle)

	srv := http.Server{Handler: mux}
	logger.Debugf("stats worker now serving")
	defer logger.Debugf("stats worker serving finished")
	defer close(w.done)
	srv.Serve(w.listener)
}

func (w *socketListener) run() error {
	defer logger.Debugf("stats worker finished")
	<-w.tomb.Dying()
	logger.Debugf("stats worker closing listener")
	w.listener.Close()
	// Don't mark the worker as done until the serve goroutine has finished.
	<-w.done
	return nil
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
	PubSub             IntrospectionReporter
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	Presence           presence.Recorder
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
	handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	handle("/depengine", depengineHandler{sources.DependencyEngine})
	handle("/statepool", introspectionReporterHandler{
		name:     "State Pool Report",
		reporter: sources.StatePool,
	})
	handle("/pubsub", introspectionReporterHandler{
		name:     "PubSub Report",
		reporter: sources.PubSub,
	})
	handle("/metrics/", promhttp.HandlerFor(sources.PrometheusGatherer, promhttp.HandlerOpts{}))
	// Unit agents don't have a presence recorder to pass in.
	if sources.Presence != nil {
		handle("/presence/", presenceHandler{sources.Presence})
	}
	handle("/machinelock/", machineLockHandler{sources.MachineLock})
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

type machineLockHandler struct {
	lock machinelock.Lock
}

// ServeHTTP is part of the http.Handler interface.
func (h machineLockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.lock == nil {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintln(w, "missing machine lock reporter")
		return
	}
	var args []machinelock.ReportOption
	q := r.URL.Query()
	if v := q.Get("yaml"); v != "" {
		args = append(args, machinelock.ShowDetailsYAML)
	}
	if v := q.Get("history"); v != "" {
		args = append(args, machinelock.ShowHistory)
	}
	if v := q.Get("stack"); v != "" {
		args = append(args, machinelock.ShowStack)
	}

	content, err := h.lock.Report(args...)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprintf(w, "error: %v\n", err)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, content)
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

type presenceHandler struct {
	presence presence.Recorder
}

// ServeHTTP is part of the http.Handler interface.
func (h presenceHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.presence == nil || !h.presence.IsEnabled() {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("agent is not an apiserver\n"))
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	tw := output.TabWriter(w)
	wrapper := output.Wrapper{tw}

	// Could be smart here and switch on the request accept header.
	connections := h.presence.Connections()
	models := connections.Models()
	sort.Strings(models)

	for _, name := range models {
		wrapper.Println("[" + name + "]")
		wrapper.Println()
		wrapper.Println("AGENT", "SERVER", "CONN ID", "STATUS")
		values := connections.ForModel(name).Values()
		sort.Sort(ValueSort(values))
		for _, value := range values {
			agentName := value.Agent
			if value.ControllerAgent {
				agentName += " (controller)"
			}
			wrapper.Println(agentName, value.Server, value.ConnectionID, value.Status)
		}
		wrapper.Println()
	}
	tw.Flush()
}

type ValueSort []presence.Value

func (a ValueSort) Len() int      { return len(a) }
func (a ValueSort) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ValueSort) Less(i, j int) bool {
	// Sort by agent, then server, then connection id
	if a[i].Agent != a[j].Agent {
		return a[i].Agent < a[j].Agent
	}
	if a[i].Server != a[j].Server {
		return a[i].Server < a[j].Server
	}
	return a[i].ConnectionID < a[j].ConnectionID
}
