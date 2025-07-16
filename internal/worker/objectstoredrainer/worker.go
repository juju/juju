// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstoredrainer

import (
	"context"
	"io"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
	"github.com/juju/juju/internal/worker/fortress"
)

// SelectFileHashFunc is a function that selects the file hash from the
// metadata.
type SelectFileHashFunc func(objectstore.Metadata) string

// HashFileSystemAccessor is the interface for reading and deleting files from
// the file system.
// The file system accessor is used for draining files from the file backed
// object store to the s3 object store. It should at no point be used for
// writing files to the file system.
type HashFileSystemAccessor interface {
	// HashExists checks if the file exists in the file backed object store.
	// Returns a NotFound error if the file doesn't exist.
	HashExists(ctx context.Context, hash string) error

	// GetByHash returns an io.ReadCloser for the file at the given hash.
	GetByHash(ctx context.Context, hash string) (io.ReadCloser, int64, error)

	// DeleteByHash deletes the file at the given hash.
	DeleteByHash(ctx context.Context, hash string) error
}

// NewHashFileSystemAccessorFunc is a function that creates a new
// HashFileSystemAccessor.
type NewHashFileSystemAccessorFunc func(namespace, rootDir string, logger logger.Logger) HashFileSystemAccessor

// GuardService provides access to the object store for draining
// operations.
type GuardService interface {
	// GetDrainingPhase returns the current active draining phase of the
	// object store.
	GetDrainingPhase(ctx context.Context) (objectstore.Phase, error)
	// WatchDraining returns a watcher that watches the draining phase of the
	// object store.
	WatchDraining(ctx context.Context) (watcher.Watcher[struct{}], error)
}

// ControllerService provides access to the controller for draining
// operations.
type ControllerService interface {
	// GetModelNamespaces returns the model namespaces of all models in the
	// state.
	GetModelNamespaces(ctx context.Context) ([]string, error)
}

// Config holds the dependencies and configuration for a Worker.
type Config struct {
	Guard                     fortress.Guard
	GuardService              GuardService
	ControllerService         ControllerService
	NewHashFileSystemAccessor NewHashFileSystemAccessorFunc
	S3Client                  objectstore.Client
	SelectFileHash            SelectFileHashFunc
	Logger                    logger.Logger
	Clock                     clock.Clock
}

// Validate returns an error if the config cannot be expected to
// drive a functional Worker.
func (config Config) Validate() error {
	if config.Guard == nil {
		return errors.Errorf("nil Guard").Add(coreerrors.NotValid)
	}
	if config.GuardService == nil {
		return errors.Errorf("nil GuardService").Add(coreerrors.NotValid)
	}
	if config.ControllerService == nil {
		return errors.Errorf("nil ControllerService").Add(coreerrors.NotValid)
	}
	if config.NewHashFileSystemAccessor == nil {
		return errors.Errorf("nil NewHashFileSystemAccessor").Add(coreerrors.NotValid)
	}
	if config.S3Client == nil {
		return errors.Errorf("nil S3Client").Add(coreerrors.NotValid)
	}
	if config.SelectFileHash == nil {
		return errors.Errorf("nil SelectFileHash").Add(coreerrors.NotValid)
	}
	if config.Logger == nil {
		return errors.Errorf("nil Logger").Add(coreerrors.NotValid)
	}
	if config.Clock == nil {
		return errors.Errorf("nil Clock").Add(coreerrors.NotValid)
	}
	return nil
}

// NewWorker returns a Worker that tracks the result of the configured.
func NewWorker(ctx context.Context, config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Capture(err)
	}

	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "objectstore-drainer",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return true
		},
		RestartDelay: time.Second * 10,
		Clock:        config.Clock,
		Logger:       internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		guard:        config.Guard,
		guardService: config.GuardService,

		controllerService: config.ControllerService,

		newFileSystem: config.NewHashFileSystemAccessor,
		client:        config.S3Client,

		selectFileHash: config.SelectFileHash,

		runner: runner,

		logger: config.Logger,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "objectstoredrainer",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{
			runner,
		},
	}); err != nil {
		return nil, errors.Capture(err)
	}
	return w, nil
}

// Worker watches the object store service for changes to the draining
// phase. If the phase is draining, it locks the guard. If the phase is not
// draining, it unlocks the guard.
// The worker will manage the lifecycle of the watcher and will stop
// watching when the worker is killed or when the context is cancelled.
type Worker struct {
	catacomb catacomb.Catacomb

	guard        fortress.Guard
	guardService GuardService

	controllerService ControllerService

	newFileSystem NewHashFileSystemAccessorFunc
	client        objectstore.Client

	selectFileHash SelectFileHashFunc

	runner worker.Runner

	logger logger.Logger
}

// Kill kills the worker. It will cause the worker to stop if it is
// not already stopped. The worker will transition to the dying state.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. It will cause the worker to
// stop if it is not already stopped. It will return an error if the
// worker was killed with an error.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report returns a report of the worker's state. This is used for
// debugging and monitoring purposes.
func (w *Worker) Report() map[string]any {
	return w.runner.Report()
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.guardService.WatchDraining(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Capture(err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case <-watcher.Changes():
			phase, err := w.guardService.GetDrainingPhase(ctx)
			if err != nil {
				return errors.Capture(err)
			}

			// We're not draining, so we can unlock the guard and wait
			// for the next change.
			if !phase.IsDraining() {
				w.logger.Infof(ctx, "object store is not draining, unlocking guard")

				if err := w.guard.Unlock(ctx); err != nil {
					return errors.Errorf("failed to update guard: %v", err)
				}
				continue
			}

			w.logger.Infof(ctx, "object store is draining, locking guard")

			if err := w.guard.Lockdown(ctx); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}

			// TODO (stickupkid): Support draining from one s3 object store to
			// another. For now, we just log that we're in the draining phase
			// from file to s3.

			namespaces, err := w.controllerService.GetModelNamespaces(ctx)
			if err != nil {
				return errors.Errorf("getting model namespaces: %w", err)
			}

			if err := w.drainModels(ctx, namespaces); err != nil {
				return errors.Errorf("draining models: %w", err)
			}

			w.logger.Infof(ctx, "object store draining complete, unlocking guard")

			if err := w.guard.Unlock(ctx); err != nil {
				return errors.Errorf("failed to update guard: %v", err)
			}
		}
	}
}

func (w *Worker) drainModels(ctx context.Context, namespaces []string) error {
	for _, namespace := range namespaces {
		w.logger.Infof(ctx, "draining model %q", namespace)

		err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
			fileSystem := w.newFileSystem(namespace, w.client.RootDir(), w.logger)
			return newDrainWorker(
				fileSystem,
				w.client,
				metadataService,
				rootBucket,
				namespace,
				w.selectFileHash,
				w.logger,
			), nil
		})
		if errors.Is(err, coreerrors.AlreadyExists) {
			continue
		} else if err != nil {
			return errors.Errorf("starting worker for model %q: %w", namespace, err)
		}
	}
	return nil
}
