// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package introspection

import (
	"fmt"
	"net"
	"net/http"
	"runtime"
	"sort"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gopkg.in/tomb.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/machinelock"
	"github.com/juju/juju/core/output"
	"github.com/juju/juju/core/presence"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/pubsub/agent"
	"github.com/juju/juju/internal/worker/introspection/pprof"
	"github.com/juju/juju/juju/sockets"
)

var logger = internallogger.GetLogger("juju.worker.introspection")

// DepEngineReporter provides insight into the running dependency engine of the agent.
type DepEngineReporter interface {
	// Report returns a map describing the state of the receiver. It is expected
	// to be goroutine-safe.
	Report() map[string]interface{}
}

// Reporter provides a simple method that the introspection
// worker will output for the entity.
type Reporter interface {
	IntrospectionReport() string
}

// Clock represents the ability to wait for a bit.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// SimpleHub is a pubsub hub used for internal messaging.
type SimpleHub interface {
	Publish(topic string, data interface{}) func()
	Subscribe(topic string, handler func(string, interface{})) func()
}

// StructuredHub is a pubsub hub used for messaging within the HA
// controller applications.
type StructuredHub interface {
	Publish(topic string, data interface{}) (func(), error)
	Subscribe(topic string, handler interface{}) (func(), error)
}

// Config describes the arguments required to create the introspection worker.
type Config struct {
	SocketName         string
	DepEngine          DepEngineReporter
	StatePool          Reporter
	PubSub             Reporter
	MachineLock        machinelock.Lock
	PrometheusGatherer prometheus.Gatherer
	Presence           presence.Recorder
	Clock              Clock
	LocalHub           SimpleHub
	CentralHub         StructuredHub
}

// Validate checks the config values to assert they are valid to create the worker.
func (c *Config) Validate() error {
	if c.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if c.PrometheusGatherer == nil {
		return errors.NotValidf("nil PrometheusGatherer")
	}
	if c.LocalHub != nil && c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	return nil
}

// socketListener is a worker and constructed with NewWorker.
type socketListener struct {
	tomb               tomb.Tomb
	listener           net.Listener
	depEngine          DepEngineReporter
	statePool          Reporter
	pubsub             Reporter
	machineLock        machinelock.Lock
	prometheusGatherer prometheus.Gatherer
	presence           presence.Recorder
	clock              Clock
	localHub           SimpleHub
	centralHub         StructuredHub
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
	l, err := sockets.Listen(sockets.Socket{

		Network: "unix",
		Address: config.SocketName,
	})
	if err != nil {
		return nil, errors.Annotate(err, "unable to listen on unix socket")
	}
	logger.Debugf("introspection worker listening on %q", config.SocketName)

	w := &socketListener{
		listener:           l,
		depEngine:          config.DepEngine,
		statePool:          config.StatePool,
		pubsub:             config.PubSub,
		machineLock:        config.MachineLock,
		prometheusGatherer: config.PrometheusGatherer,
		presence:           config.Presence,
		clock:              config.Clock,
		localHub:           config.LocalHub,
		centralHub:         config.CentralHub,
		done:               make(chan struct{}),
	}
	go w.serve()
	w.tomb.Go(w.run)
	return w, nil
}

func (w *socketListener) serve() {
	mux := http.NewServeMux()
	w.RegisterHTTPHandlers(mux.Handle)

	srv := http.Server{Handler: mux}
	logger.Debugf("stats worker now serving")
	defer logger.Debugf("stats worker serving finished")
	defer close(w.done)
	_ = srv.Serve(w.listener)
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

// RegisterHTTPHandlers calls the given function with http.Handlers
// that serve agent introspection requests. The function will
// be called with a path; the function may alter the path
// as it sees fit.
func (w *socketListener) RegisterHTTPHandlers(
	handle func(path string, h http.Handler),
) {
	handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	handle("/depengine", depengineHandler{w.depEngine})
	handle("/metrics", promhttp.HandlerFor(w.prometheusGatherer, promhttp.HandlerOpts{}))
	handle("/machinelock", machineLockHandler{w.machineLock})
	// The trailing slash is kept for metrics because we don't want to
	// break the metrics exporting that is using the internal charm. Since
	// we don't know if it is using the exported shell function, or calling
	// the introspection endpoint directly.
	handle("/metrics/", promhttp.HandlerFor(w.prometheusGatherer, promhttp.HandlerOpts{}))

	// Only machine or controller agents support the following.
	if w.statePool != nil {
		handle("/statepool", introspectionReporterHandler{
			name:     "State Pool Report",
			reporter: w.statePool,
		})
	} else {
		handle("/statepool", notSupportedHandler{"State Pool"})
	}
	if w.pubsub != nil {
		handle("/pubsub", introspectionReporterHandler{
			name:     "PubSub Report",
			reporter: w.pubsub,
		})
	} else {
		handle("/pubsub", notSupportedHandler{"PubSub Report"})
	}
	if w.presence != nil {
		handle("/presence", presenceHandler{w.presence})
	} else {
		handle("/presence", notSupportedHandler{"Presence"})
	}
	if w.localHub != nil {
		handle("/units", unitsHandler{w.clock, w.localHub, w.done})
	} else {
		handle("/units", notSupportedHandler{"Units"})
	}
	// TODO(leases) - add metrics
	handle("/leases", notSupportedHandler{"Leases"})
}

type notSupportedHandler struct {
	name string
}

// ServeHTTP is part of the http.Handler interface.
func (h notSupportedHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	http.Error(w, fmt.Sprintf("%q introspection not supported", h.name), http.StatusNotFound)
}

type depengineHandler struct {
	reporter DepEngineReporter
}

// ServeHTTP is part of the http.Handler interface.
func (h depengineHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.reporter == nil {
		http.Error(w, "missing dependency engine reporter", http.StatusNotFound)
		return
	}
	bytes, err := yaml.Marshal(h.reporter.Report())
	if err != nil {
		http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	fmt.Fprint(w, "Dependency Engine Report\n\n")
	_, _ = w.Write(bytes)
}

type machineLockHandler struct {
	lock machinelock.Lock
}

// ServeHTTP is part of the http.Handler interface.
func (h machineLockHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.lock == nil {
		http.Error(w, "missing machine lock reporter", http.StatusNotFound)
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
		http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	fmt.Fprint(w, content)
}

type introspectionReporterHandler struct {
	name     string
	reporter Reporter
}

// ServeHTTP is part of the http.Handler interface.
func (h introspectionReporterHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.reporter == nil {
		http.Error(w, fmt.Sprintf("%s: missing reporter", h.name), http.StatusNotFound)
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
		http.Error(w, "agent is not an apiserver", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	tw := output.TabWriter(w)
	wrapper := output.Wrapper{TabWriter: tw}

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

type unitsHandler struct {
	clock Clock
	hub   SimpleHub
	done  <-chan struct{}
}

// ServeHTTP is part of the http.Handler interface.
func (h unitsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	switch action := r.Form.Get("action"); action {
	case "":
		http.Error(w, "missing action", http.StatusBadRequest)
	case "start":
		h.publishUnitsAction(w, r, "start", agent.StartUnitTopic, agent.StartUnitResponseTopic)
	case "stop":
		h.publishUnitsAction(w, r, "stop", agent.StopUnitTopic, agent.StopUnitResponseTopic)
	case "status":
		h.status(w, r)
	default:
		http.Error(w, fmt.Sprintf("unknown action: %q", action), http.StatusBadRequest)
	}
}

func (h unitsHandler) publishUnitsAction(w http.ResponseWriter, r *http.Request,
	action, topic, responseTopic string) {
	if r.Method != http.MethodPost {
		http.Error(w, fmt.Sprintf("%s requires a POST request, got %q", action, r.Method), http.StatusMethodNotAllowed)
		return
	}

	units := r.Form["unit"]
	if len(units) == 0 {
		http.Error(w, "missing unit", http.StatusBadRequest)
		return
	}

	h.publishAndAwaitResponse(w, topic, responseTopic, agent.Units{Names: units})
}

func (h unitsHandler) status(w http.ResponseWriter, r *http.Request) {
	h.publishAndAwaitResponse(w, agent.UnitStatusTopic, agent.UnitStatusResponseTopic, nil)
}

func (h unitsHandler) publishAndAwaitResponse(w http.ResponseWriter, topic, responseTopic string, data interface{}) {
	response := make(chan interface{})
	unsubscribe := h.hub.Subscribe(responseTopic, func(topic string, body interface{}) {
		select {
		case response <- body:
		case <-h.done:
		}
	})
	defer unsubscribe()

	h.hub.Publish(topic, data)

	select {
	case message := <-response:
		bytes, err := yaml.Marshal(message)
		if err != nil {
			http.Error(w, fmt.Sprintf("error: %v", err), http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(bytes)
	case <-h.done:
		http.Error(w, "introspection worker stopping", http.StatusServiceUnavailable)
	case <-h.clock.After(10 * time.Second):
		http.Error(w, "response timed out", http.StatusInternalServerError)
	}
}
