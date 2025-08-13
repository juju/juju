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
	"github.com/juju/juju/internal/errors"
)

// Registry holds all the watchers for a connection.
// It allows the registration of watchers that will be cleaned up when a
// connection terminates.
type Registry struct {
	catacomb                  catacomb.Catacomb
	runner                    *worker.Runner
	namespaceCounter, counter int64
}

// NewRegistry returns a new Registry that also starts a worker to manage the
// registry.
func NewRegistry(clock clock.Clock) (*Registry, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "watcher-registry",
		// Prevent the runner from restarting the worker, if one of the
		// workers dies, we want to stop the whole thing.
		IsFatal:       func(err error) bool { return false },
		ShouldRestart: func(err error) bool { return false },
		Clock:         clock,
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	r := &Registry{
		runner: runner,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "watcher-registry",
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

	err := r.register(context.TODO(), namespace, w)
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

	return r.register(context.TODO(), namespace, w)
}

func (r *Registry) register(ctx context.Context, namespace string, w worker.Worker) error {
	err := r.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		return w, nil
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
