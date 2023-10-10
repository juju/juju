// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controlleragentconfig

import (
	"context"
	"fmt"
	"os"
	"sync/atomic"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"
)

const (
	// States which report the state of the worker.
	stateStarted = "started"
	stateReload  = "reload"
)

// WorkerConfig encapsulates the configuration options for the
// agent controller config worker.
type WorkerConfig struct {
	Logger Logger
	Notify func(context.Context, chan os.Signal)
	Clock  clock.Clock
}

// Validate ensures that the config values are valid.
func (c *WorkerConfig) Validate() error {
	if c.Logger == nil {
		return errors.NotValidf("nil Logger")
	}
	if c.Notify == nil {
		return errors.NotValidf("nil Notify")
	}
	if c.Clock == nil {
		return errors.NotValidf("nil Clock")
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
	internalStates chan string
	cfg            WorkerConfig
	catacomb       catacomb.Catacomb
	runner         *worker.Runner
	unique         int64
}

// NewWorker creates a new tracer worker.
func NewWorker(cfg WorkerConfig) (*configWorker, error) {
	return newWorker(cfg, nil)
}

func newWorker(cfg WorkerConfig, internalStates chan string) (*configWorker, error) {
	var err error
	if err = cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &configWorker{
		internalStates: internalStates,
		cfg:            cfg,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: cfg.Clock,
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return false
			},
			Logger: cfg.Logger,
		}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Trace(err)
	}

	return w, nil
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
	unique := atomic.AddInt64(&w.unique, 1)
	namespace := fmt.Sprintf("watcher-%d", unique)
	err := w.runner.StartWorker(namespace, func() (worker.Worker, error) {
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

func (w *configWorker) loop() error {
	// We must use a buffered channel or risk missing the signal
	// if we're not ready to receive when the signal is sent.
	ch := make(chan os.Signal, 1)
	w.cfg.Notify(w.catacomb.Context(context.Background()), ch)

	// Report the initial started state.
	w.reportInternalState(stateStarted)

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.Err()
		case <-ch:
			w.reportInternalState(stateReload)

			w.cfg.Logger.Infof("SIGHUP received, reloading config")

			for _, name := range w.runner.WorkerNames() {
				runnerWorker, err := w.runner.Worker(name, w.catacomb.Dying())
				if err != nil {
					if errors.Is(err, errors.NotFound) {
						w.cfg.Logger.Debugf("worker %q not found, skipping", name)
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
