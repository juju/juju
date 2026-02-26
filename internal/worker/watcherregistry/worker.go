// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcherregistry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
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

	// Report returns a map of the current state of the registry.
	Report() map[string]any
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

	namespacePrefix string

	clock  clock.Clock
	logger logger.Logger
}

// NewWorker creates a new watcher registry worker.
func NewWorker(config Config) (worker.Worker, error) {
	// Generate a random namespace prefix to avoid collisions when the process
	// is restarted and the consuming side doesn't know about the restart and
	// attempts to register a watcher with the same name. This might not be
	// the same watcher as before and will cause cryptic errors.
	token := make([]byte, 4)
	if _, err := rand.Read(token); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "watcher-registry",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Clock:  config.Clock,
		Logger: &runnerLogger{logger: config.Logger},
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		namespacePrefix: hex.EncodeToString(token),

		runner: runner,

		clock:  config.Clock,
		logger: config.Logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "watcher-registry",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{runner},
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

// GetWatcherRegistry returns the watcher registry for a given id.
func (w *Worker) GetWatcherRegistry(ctx context.Context, id uint64) (WatcherRegistry, error) {
	// First attempt to get the registry without attempting to start a new
	// worker flow. If the worker is not found, then start the worker.
	sid := strconv.FormatUint(id, 10)
	wrk, err := w.runner.Worker(sid, w.catacomb.Dying())
	if err != nil && !errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Capture(err)
	} else if err == nil {
		return wrk.(WatcherRegistry), nil
	}

	err = w.runner.StartWorker(ctx, sid, func(ctx context.Context) (worker.Worker, error) {
		tw, err := newTrackedWorker("watcher-registry"+sid, w.namespacePrefix, w.clock, w.logger)
		if err != nil {
			return nil, errors.Capture(err)
		}
		return tw, nil
	})
	if err != nil && !errors.Is(err, coreerrors.AlreadyExists) {
		return nil, errors.Capture(err)
	}

	// The worker is started, but we need to get it so we can return it.
	wrk, err = w.runner.Worker(sid, w.catacomb.Dying())
	if err != nil {
		return nil, errors.Capture(err)
	}
	return wrk.(WatcherRegistry), nil
}

func (w *Worker) loop() error {
	<-w.catacomb.Dying()
	return w.catacomb.ErrDying()
}

type trackedWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner

	namespacePrefix  string
	namespaceCounter int64
}

func newTrackedWorker(name, namespacePrefix string, clock clock.Clock, logger logger.Logger) (*trackedWorker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: name,
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return false
		},
		Clock:  clock,
		Logger: &runnerLogger{logger: logger},
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	reg := &trackedWorker{
		runner: runner,

		namespacePrefix:  namespacePrefix,
		namespaceCounter: 0,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: name,
		Site: &reg.catacomb,
		Work: reg.loop,
		Init: []worker.Worker{runner},
	}); err != nil {
		return nil, errors.Capture(err)
	}

	return reg, nil
}

// Get returns the watcher for the given id, or nil if there is no such
// watcher.
func (r *trackedWorker) Get(id string) (worker.Worker, error) {
	select {
	case <-r.catacomb.Dying():
		return nil, errors.Errorf("watcher %q %w", id, coreerrors.NotFound).
			Add(ErrWatcherRegistryClosed)
	default:
	}

	w, err := r.runner.Worker(id, r.catacomb.Dying())
	if errors.Is(err, coreerrors.NotFound) {
		return nil, errors.Errorf("11 watcher %q %w", id, coreerrors.NotFound)
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

	if err := r.register(ctx, namespace, w); err != nil {
		return "", errors.Capture(err)
	}
	return namespace, nil
}

// RegisterNamed registers the given watcher. Callers must supply a unique
// name for the given watcher. It is an error to try to register another
// watcher with the same name as an already registered name.
// It is also an error to supply a name that is an integer string, since it
// is more preferred to use the auto-naming from Register.
func (r *trackedWorker) RegisterNamed(ctx context.Context, name string, w worker.Worker) error {
	if _, err := strconv.Atoi(name); err == nil {
		return errors.Errorf("name as integer %q %w", name, coreerrors.NotValid)
	} else if strings.HasPrefix(name, r.namespacePrefix) {
		return errors.Errorf("name %q %w", name, coreerrors.NotValid)
	}

	if err := r.register(ctx, name, w); errors.Is(err, coreerrors.AlreadyExists) {
		return errors.Errorf("watcher %q %w", name, coreerrors.AlreadyExists)
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
	if err := r.runner.StopAndRemoveWorker(id, r.catacomb.Dying()); err != nil {
		return errors.Capture(err)
	}
	return nil
}

// StopAll stops all resources and unregisters them. The registry is then
// considered closed and no further registrations are allowed.
func (r *trackedWorker) StopAll() error {
	r.catacomb.Kill(nil)
	return r.catacomb.Wait()
}

// Count returns the number of resources currently held.
func (r *trackedWorker) Count() int {
	return len(r.runner.WorkerNames())
}

// Kill stops the worker and cleans up any resources.
func (r *trackedWorker) Kill() {
	r.catacomb.Kill(nil)
}

// Wait waits for the worker to finish.
func (r *trackedWorker) Wait() error {
	return r.catacomb.Wait()
}

// Report returns a map of the current state of the registry.
func (r *trackedWorker) Report() map[string]any {
	report := make(map[string]any)
	report["namespacePrefix"] = r.namespacePrefix
	report["namespaceCounter"] = r.namespaceCounter
	report["workers"] = r.runner.Report()
	return report
}

func (r *trackedWorker) loop() error {
	<-r.catacomb.Dying()
	return r.catacomb.ErrDying()
}

func (r *trackedWorker) register(ctx context.Context, namespace string, w worker.Worker) error {
	err := r.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		return w, nil
	})
	if err != nil {
		select {
		case <-r.catacomb.Dying():
			return errors.Errorf("watcher %q %w", namespace, coreerrors.NotFound).
				Add(ErrWatcherRegistryClosed)
		default:
			return errors.Capture(err)
		}
	}

	return nil
}

// runnerLogger is a logger.Logger that logs to worker.Logger interface, but
// all INFO and DEBUG messages are logged as TRACE. This is because the
// runner produces a lot of INFO and DEBUG messages that are not useful
// in the context of the watcher registry.
type runnerLogger struct {
	logger logger.Logger
}

// Errorf logs a message at the ERROR level.
func (c *runnerLogger) Errorf(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Errorf(context.Background(), msg, args...)
}

// Infof logs a message at the TRACE level.
func (c *runnerLogger) Infof(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Tracef(context.Background(), msg, args...)
}

// Debugf logs a message at the TRACE level.
func (c *runnerLogger) Debugf(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Tracef(context.Background(), msg, args...)
}

// Tracef logs a message at the TRACE level.
func (c *runnerLogger) Tracef(msg string, args ...any) {
	c.logger.Helper()
	c.logger.Tracef(context.Background(), msg, args...)
}
