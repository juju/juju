// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcherregistry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

const (
	// ErrWatcherRegistryClosed is returned when the watcher registry is closed.
	ErrWatcherRegistryClosed = errors.ConstError("watcher registry closed")
)

// WatcherRegistry holds all the watchers for a connection.
// It allows the registration of watchers that will be cleaned up when a
// connection terminates.
type WatcherRegistry interface {
	// Get returns the watcher for the given id, or nil if there is no such
	// watcher.
	Get(string) (worker.Worker, error)
	// Register registers the given watcher. It returns a unique identifier for the
	// watcher which can then be used in subsequent API requests to refer to the
	// watcher.
	Register(context.Context, worker.Worker) (string, error)

	// RegisterNamed registers the given watcher. Callers must supply a unique
	// name for the given watcher. It is an error to try to register another
	// watcher with the same name as an already registered name.
	// It is also an error to supply a name that is an integer string, since that
	// collides with the auto-naming from Register.
	RegisterNamed(context.Context, string, worker.Worker) error

	// Stop stops the resource with the given id and unregisters it.
	// It returns any error from the underlying Stop call.
	// It does not return an error if the resource has already
	// been unregistered.
	Stop(id string) error

	// StopAll stops all resources and unregisters them. The registry is then
	// considered closed and no further registrations are allowed.
	StopAll() error

	// Count returns the number of resources currently held.
	Count() int
}

// WatcherRegistryGetter defines an interface for retrieving
// WatcherRegistry instances by their ID.
type WatcherRegistryGetter interface {
	// GetWatcherRegistry returns the watcher registry for a given id.
	GetWatcherRegistry(ctx context.Context, id uint64) (WatcherRegistry, error)
}

// Config defines the configuration for the watcher registry.
type Config struct {
	Clock  clock.Clock
	Logger logger.Logger
}

// Worker is a watcher registry worker, used to collect and manage watchers
// for a connection.
type Worker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	registries map[uint64]WatcherRegistry
	requests   chan request

	namespacePrefix string
}

// NewWorker creates a new watcher registry worker.
func NewWorker(config Config) (worker.Worker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "watcher-registry",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Clock:  config.Clock,
		Logger: internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Generate a random namespace prefix to avoid collisions when the process
	// is restarted and the consuming side doesn't know about the restart and
	// attempts to register a watcher with the same name. This might not be
	// the same watcher as before and will cause cryptic errors.
	token := make([]byte, 4)
	if _, err := rand.Read(token); err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		runner: runner,

		registries: make(map[uint64]WatcherRegistry),
		requests:   make(chan request),

		namespacePrefix: hex.EncodeToString(token),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "watcher-registry",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return w, nil
}

// Kill stops the worker and cleans up any resources.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a map of the current state of the registry.
func (w *Worker) Report() map[string]any {
	return w.runner.Report()
}

type request struct {
	id       uint64
	response chan WatcherRegistry
}

// GetWatcherRegistry returns the watcher registry for a given id.
func (w *Worker) GetWatcherRegistry(ctx context.Context, id uint64) (WatcherRegistry, error) {
	ch := make(chan WatcherRegistry, 1)
	select {
	case <-w.catacomb.Dying():
		return nil, errors.Errorf("watcher registry %d %w", id, ErrWatcherRegistryClosed)
	case <-ctx.Done():
		return nil, errors.Capture(ctx.Err())

	case w.requests <- request{
		id:       id,
		response: ch,
	}:
	}

	select {
	case <-w.catacomb.Dying():
		return nil, errors.Errorf("watcher registry %d %w", id, ErrWatcherRegistryClosed)
	case <-ctx.Done():
		return nil, errors.Capture(ctx.Err())
	case reg := <-ch:
		return reg, nil
	}
}

func (w *Worker) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case req := <-w.requests:
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()

			case req.response <- w.makeRegistry(req.id):
			}
		}
	}
}

func (w *Worker) makeRegistry(id uint64) WatcherRegistry {
	// Cache the registry if it exists, otherwise create a new one.
	if reg, ok := w.registries[id]; ok {
		return reg
	}

	reg := &trackedWorker{
		id:     strconv.FormatUint(id, 10),
		runner: w.runner,
		dying:  w.catacomb.Dying(),

		namespacePrefix:  w.namespacePrefix,
		namespaceCounter: 0,
	}

	w.registries[id] = reg

	return reg
}

type trackedWorker struct {
	id     string
	runner *worker.Runner

	dying  <-chan struct{}
	closed atomic.Bool

	namespacePrefix  string
	namespaceCounter int64
}

// Get returns the watcher for the given id, or nil if there is no such
// watcher.
func (r *trackedWorker) Get(id string) (worker.Worker, error) {
	if r.closed.Load() {
		return nil, errors.Errorf("watcher %q %w", id, coreerrors.NotFound).
			Add(ErrWatcherRegistryClosed)
	}

	w, err := r.runner.Worker(r.prefixNamespace(id), r.dying)
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf("watcher %q %w", id, coreerrors.NotFound)
	} else if err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Register registers the given watcher. It returns a unique identifier for the
// watcher which can then be used in subsequent API requests to refer to the
// watcher.
func (r *trackedWorker) Register(ctx context.Context, w worker.Worker) (string, error) {
	nsCounter := atomic.AddInt64(&r.namespaceCounter, 1)
	namespace := r.namespacePrefix + "-" + strconv.FormatInt(nsCounter, 10)

	if err := r.register(ctx, namespace, w); errors.Is(err, coreerrors.NotFound) {
		return "", errors.Errorf("watcher %q %w", namespace, coreerrors.NotFound)
	} else if err != nil {
		return "", errors.Capture(err)
	}
	return namespace, nil
}

// RegisterNamed registers the given watcher. Callers must supply a unique
// name for the given watcher. It is an error to try to register another
// watcher with the same name as an already registered name.
// It is also an error to supply a name that is an integer string, since that
// collides with the auto-naming from Register.
func (r *trackedWorker) RegisterNamed(ctx context.Context, namespace string, w worker.Worker) error {
	if _, err := strconv.Atoi(namespace); err == nil {
		return errors.Errorf("namespace %q %w", namespace, coreerrors.NotValid)
	}

	if err := r.register(ctx, namespace, w); errors.Is(err, coreerrors.NotFound) {
		return errors.Errorf("watcher %q %w", namespace, coreerrors.NotFound)
	} else if errors.Is(err, coreerrors.AlreadyExists) {
		return errors.Errorf("worker %q %w", namespace, coreerrors.AlreadyExists)
	} else if err != nil {
		return errors.Capture(err)
	}
	return nil
}

// Stop stops the resource with the given id and unregisters it.
// It returns any error from the underlying Stop call.
// It does not return an error if the resource has already
// been unregistered.
func (r *trackedWorker) Stop(id string) error {
	if err := r.runner.StopAndRemoveWorker(r.prefixNamespace(id), r.dying); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// StopAll stops all resources and unregisters them. The registry is then
// considered closed and no further registrations are allowed.
func (r *trackedWorker) StopAll() error {
	// We don't care if the closed flag was already set, we just want to
	// ensure that we don't try to register any more watchers.
	_ = r.closed.Swap(true)

	for _, name := range r.runner.WorkerNames() {
		if !r.isOwnedByNamespace(name) {
			continue
		}

		if err := r.runner.StopAndRemoveWorker(name, r.dying); err != nil && !errors.Is(err, coreerrors.NotFound) {
			return errors.Capture(err)
		}
	}
	return nil
}

// Count returns the number of resources currently held.
func (r *trackedWorker) Count() int {
	var amount int
	for _, name := range r.runner.WorkerNames() {
		if !r.isOwnedByNamespace(name) {
			continue
		}
		amount++
	}
	return amount
}

func (r *trackedWorker) register(ctx context.Context, namespace string, w worker.Worker) error {
	if r.closed.Load() {
		return ErrWatcherRegistryClosed
	}

	// We don't own the worker being given to us (this is an anti-pattern),
	// so we need to ensure that the runner owns the worker before we've
	// finished registering it. There is a small window where the runner is
	// killed and the worker is not yet fully registered, causing the worker
	// passed in to the register function to be leaked.
	started := make(chan struct{})

	scopedNamespace := r.prefixNamespace(namespace)
	err := r.runner.StartWorker(ctx, scopedNamespace, func(ctx context.Context) (worker.Worker, error) {
		if r.closed.Load() {
			return nil, ErrWatcherRegistryClosed
		}

		close(started)

		return w, nil
	})
	if err != nil {
		return errors.Capture(err)
	}

	select {
	case <-r.dying:
		return errors.Errorf("watcher %q %w", namespace, ErrWatcherRegistryClosed)

	case <-ctx.Done():
		return errors.Capture(ctx.Err())

	case <-started:
	}

	return nil
}

func (r *trackedWorker) prefixNamespace(id string) string {
	return fmt.Sprintf("%s:%s", r.id, id)
}

func (r *trackedWorker) isOwnedByNamespace(id string) bool {
	return strings.HasPrefix(id, r.id+":")
}
