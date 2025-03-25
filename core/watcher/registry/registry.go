// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package registry

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	internallogger "github.com/juju/juju/internal/logger"
)

const (
	// DefaultNamespace is the default namespace for watchers.
	DefaultNamespace = "w"
)

// Option defines a function for setting options on a Registry.
type Option func(*option)

type option struct {
	logger logger.Logger
}

// WithLogger returns an Option that sets the logger to use for logging when
// workers finish.
func WithLogger(logger logger.Logger) Option {
	return func(o *option) {
		o.logger = logger
	}
}

func newOptions() *option {
	return &option{
		logger: internallogger.GetLogger("juju.core.watcher.registry"),
	}
}

type watcherUnwrapper interface {
	Unwrap() worker.Worker
}

// Registry holds all the watchers for a connection.
// It allows the registration of watchers that will be cleaned up when a
// connection terminates.
type Registry struct {
	catacomb                  catacomb.Catacomb
	runner                    *worker.Runner
	namespaceCounter, counter int64
	watcherWrapper            func(worker.Worker) (worker.Worker, error)
}

// NewRegistry returns a new Registry that also starts a worker to manage the
// registry.
func NewRegistry(clock clock.Clock, opts ...Option) (*Registry, error) {
	o := newOptions()
	for _, opt := range opts {
		opt(o)
	}

	r := &Registry{
		runner: worker.NewRunner(worker.RunnerParams{
			// Prevent the runner from restarting the worker, if one of the
			// workers dies, we want to stop the whole thing.
			IsFatal:       func(err error) bool { return false },
			ShouldRestart: func(err error) bool { return false },
			Clock:         clock,
		}),
		watcherWrapper: watcherLogDecorator(o.logger),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Site: &r.catacomb,
		Work: r.loop,
		Init: []worker.Worker{
			r.runner,
		},
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return r, nil
}

// Get returns the watcher for the given id, or nil if there is no such
// watcher.
func (r *Registry) Get(id string) (worker.Worker, error) {
	w, err := r.runner.Worker(id, r.catacomb.Dying())
	if err != nil {
		return nil, errors.Capture(err)
	}
	if lw, ok := w.(watcherUnwrapper); ok {
		return lw.Unwrap(), nil
	}
	return w, nil
}

// Register registers the given watcher. It returns a unique identifier for the
// watcher which can then be used in subsequent API requests to refer to the
// watcher.
func (r *Registry) Register(w worker.Worker) (string, error) {
	nsCounter := atomic.AddInt64(&r.namespaceCounter, 1)
	namespace := fmt.Sprintf("%s-%d", DefaultNamespace, nsCounter)

	err := r.register(namespace, w)
	if err != nil {
		return "", errors.Capture(err)
	}
	return namespace, nil
}

// RegisterNamed registers the given watcher. Callers must supply a unique
// name for the given watcher. It is an error to try to register another
// watcher with the same name as an already registered name.
// It is also an error to supply a name that is an integer string, since that
// collides with the auto-naming from Register.
func (r *Registry) RegisterNamed(namespace string, w worker.Worker) error {
	if _, err := strconv.Atoi(namespace); err == nil {
		return errors.Errorf("namespace %q %w", namespace, coreerrors.NotValid)
	}

	return r.register(namespace, w)
}

func (r *Registry) register(namespace string, w worker.Worker) error {
	err := r.runner.StartWorker(namespace, func() (worker.Worker, error) {
		return r.watcherWrapper(w)
	})
	if err != nil {
		return errors.Capture(err)
	}
	atomic.AddInt64(&r.counter, 1)
	return nil
}

// Stop stops the resource with the given id and unregisters it.
// It returns any error from the underlying Stop call.
// It does not return an error if the resource has already
// been unregistered.
func (r *Registry) Stop(id string) error {
	if err := r.runner.StopAndRemoveWorker(id, r.catacomb.Dying()); err != nil {
		return errors.Capture(err)
	}
	atomic.AddInt64(&r.counter, -1)
	return nil
}

// Kill implements the worker.Worker interface.
func (r *Registry) Kill() {
	r.catacomb.Kill(nil)
	atomic.StoreInt64(&r.counter, 0)
}

// Wait implements the worker.Worker interface.
func (r *Registry) Wait() error {
	return r.catacomb.Wait()
}

// Count returns the number of resources currently held.
func (r *Registry) Count() int {
	return int(atomic.LoadInt64(&r.counter))
}

func (r *Registry) loop() error {
	<-r.catacomb.Dying()
	return r.catacomb.ErrDying()
}

// watcherLogDecorator returns a function that wraps a worker.Worker with a
// LoggingWatcher.
func watcherLogDecorator(l logger.Logger) func(worker.Worker) (worker.Worker, error) {
	return func(w worker.Worker) (worker.Worker, error) {
		if l == nil {
			return w, nil
		}
		if l.IsLevelEnabled(logger.TRACE) {
			l.Tracef(context.TODO(), "starting watcher %T", w)
		}
		return NewLoggingWatcher(w, l), nil
	}
}

// LoggingWatcher is a wrapper around a worker.Worker that logs when it finishes.
type LoggingWatcher struct {
	worker worker.Worker
	logger logger.Logger
}

// NewLoggingWatcher returns a new LoggingWatcher that wraps the given worker,
// so we can log when it starts and finishes.
func NewLoggingWatcher(w worker.Worker, logger logger.Logger) *LoggingWatcher {
	return &LoggingWatcher{
		worker: w,
		logger: logger,
	}
}

// Kill asks the worker to stop and returns immediately.
func (l *LoggingWatcher) Kill() {
	if l.logger.IsLevelEnabled(logger.TRACE) {
		l.logger.Tracef(context.TODO(), "killing watcher %T", l.worker)
	}
	l.worker.Kill()
}

// Wait waits for the worker to complete and returns any
// error encountered when it was running or stopping.
func (l *LoggingWatcher) Wait() error {
	err := l.worker.Wait()
	if l.logger.IsLevelEnabled(logger.TRACE) {
		l.logger.Tracef(context.TODO(), "watcher %T finished with error %v", l.worker, err)
	}
	return errors.Capture(err)
}

// Unwrap returns the wrapped worker.
func (l *LoggingWatcher) Unwrap() worker.Worker {
	return l.worker
}
