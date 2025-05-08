// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gorilla/mux"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/socketlistener"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateReload  = "reload"
)

// WorkerConfig encapsulates the configuration options for the
// agent controller config worker.
type WorkerConfig struct {
	// ControllerID is the ID of this controller.
	ControllerID string
	// Logger writes log messages.
	Logger logger.Logger
	// Clock is needed for worker.NewRunner.
	Clock clock.Clock
	// SocketName is the socket file descriptor.
	SocketName string
	// NewSocketListener is the function that creates a new socket listener.
	NewSocketListener func(socketlistener.Config) (SocketListener, error)
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
	}
	if c.SocketName == "" {
		return errors.NotValidf("empty SocketName")
	}
	if c.NewSocketListener == nil {
		return errors.NotValidf("nil NewSocketListener func")
	}
	return nil
}

// ConfigWatcher is an interface that can be used to watch for changes to the
// agent controller config.
type ConfigWatcher interface {
	// Changes returns a channel that will dispatch changes when the agent
	// controller config changes.
	// The channel will be closed when the subscription is terminated or
	// the underlying worker is killed.
	Changes() <-chan struct{}
	// Done returns a channel that will be closed when the subscription is
	// closed.
	Done() <-chan struct{}
	// Unsubscribe unsubscribes the watcher from the agent controller config
	// changes.
	Unsubscribe()
}

type configWorker struct {
	internalStates  chan string
	catacomb        catacomb.Catacomb
	runner          *worker.Runner
	cfg             WorkerConfig
	reloadRequested chan struct{}
	unique          int64
}

// NewWorker creates a new config worker.
func NewWorker(cfg WorkerConfig) (*configWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*configWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name:  "controller-agent-config",
		Clock: cfg.Clock,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Logger: internalworker.WrapLogger(cfg.Logger),
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	w := &configWorker{
		internalStates:  internalStates,
		cfg:             cfg,
		reloadRequested: make(chan struct{}),
		runner:          runner,
	}

	sl, err := cfg.NewSocketListener(socketlistener.Config{
		Logger:           cfg.Logger,
		SocketName:       cfg.SocketName,
		RegisterHandlers: w.registerHandlers,
		ShutdownTimeout:  500 * time.Millisecond,
	})
	if err != nil {
		return nil, errors.Annotate(err, "controller agent config reload socket listener setup:")
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "controller-agent-config",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
			sl,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
}

func (w *configWorker) registerHandlers(r *mux.Router) {
	r.HandleFunc("/reload", w.reloadHandler).
		Methods(http.MethodPost)
	r.HandleFunc("/agent-id", w.idHandler).
		Methods(http.MethodGet)
}

// reloadHandler sends a signal to the configWorker when a config reload is
// requested.
func (w *configWorker) reloadHandler(resp http.ResponseWriter, req *http.Request) {
	select {
	case <-w.catacomb.Dying():
		resp.WriteHeader(http.StatusInternalServerError)
	case <-req.Context().Done():
		resp.WriteHeader(http.StatusInternalServerError)
	case w.reloadRequested <- struct{}{}:
		resp.WriteHeader(http.StatusNoContent)
	}
}

// idHandler simply returns this agent's ID.
// It is used by the *unit* to get the *controller's* ID.
func (w *configWorker) idHandler(resp http.ResponseWriter, req *http.Request) {
	resp.Header().Set("Content-Type", "application/text")

	_, err := resp.Write([]byte(w.cfg.ControllerID))
	if err != nil {
		w.cfg.Logger.Errorf(req.Context(), "error writing HTTP response: %v", err)
	}
}

// Kill is part of the worker.Worker interface.
func (w *configWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *configWorker) Wait() error {
	return w.catacomb.Wait()
}

// Watcher returns a Watcher for watching for changes to the agent
// controller config.
func (w *configWorker) Watcher() (ConfigWatcher, error) {
	ctx := w.catacomb.Context(context.Background())

	unique := atomic.AddInt64(&w.unique, 1)
	namespace := fmt.Sprintf("watcher-%d", unique)
	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		return newSubscription(), nil
	})
	if err != nil {
		return nil, err
	}
	watcher, err := w.runner.Worker(namespace, w.catacomb.Dying())
	if err != nil {
		return nil, err
	}
	return watcher.(ConfigWatcher), nil
}

// loop listens for a reload request picked up by the socket listener and
// restarts all subscribed workers watching the config.
func (w *configWorker) loop() error {
	// Report the initial started state.
	w.reportInternalState(stateStarted)

	ctx, cancel := w.scopedContext()
	defer cancel()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.Err()
		case <-w.reloadRequested:
			w.reportInternalState(stateReload)

			w.cfg.Logger.Infof(ctx, "reload config request received, reloading config")

			for _, name := range w.runner.WorkerNames() {
				runnerWorker, err := w.runner.Worker(name, w.catacomb.Dying())
				if err != nil {
					if errors.Is(err, errors.NotFound) {

						w.cfg.Logger.Debugf(ctx, "worker %q not found, skipping", name)
						continue
					}
					// If the runner is dead, we should stop.
					if errors.Is(err, worker.ErrDead) {
						return nil
					}
					return errors.Trace(err)
				}

				// This should ALWAYS be a subscription.
				sub := runnerWorker.(*subscription)
				sub.dispatch()
			}
		}
	}
}

func (w *configWorker) reportInternalState(state string) {
	select {
	case <-w.catacomb.Dying():
	case w.internalStates <- state:
	default:
	}
}

func (w *configWorker) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

type subscription struct {
	tomb tomb.Tomb
	in   chan struct{}
	out  chan struct{}
}

func newSubscription() *subscription {
	w := &subscription{
		in:  make(chan struct{}),
		out: make(chan struct{}),
	}
	w.tomb.Go(w.loop)
	return w
}

// Kill is part of the worker.Worker interface.
func (w *subscription) Kill() {
	w.tomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *subscription) Wait() error {
	return w.tomb.Wait()
}

// Changes returns a channel that will be closed when the agent
func (w *subscription) Changes() <-chan struct{} {
	return w.out
}

// Done returns a channel that will be closed when the subscription is
// killed.
func (w *subscription) Done() <-chan struct{} {
	return w.tomb.Dying()
}

// Unsubscribe stops the subscription from sending changes.
func (w *subscription) Unsubscribe() {
	w.tomb.Kill(nil)
}

func (w *subscription) loop() error {
	defer close(w.out)

	var out chan<- struct{}

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.in:
			out = w.out
		case out <- struct{}{}:
			out = nil
		}
	}
}

// Dispatch sends a change notification to the subscription, but doesn't block
// on the subscription being killed.
func (w *subscription) dispatch() {
	select {
	case <-w.tomb.Dying():
		return
	case w.in <- struct{}{}:
	}
}
