// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldbdeleter

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// Config holds the configuration for the model DB deleter worker.
type Config struct {
	DBDeleter       coredatabase.DBDeleter
	DeletionService ModelDatabaseDeletionService
	Logger          logger.Logger
	Clock           clock.Clock
}

// Worker deletes the dqlite databases of models that have been purged from
// this controller. It watches for staged deletions, then deletes each staged
// database, retrying on failure until the staged row is gone.
type Worker struct {
	catacomb        catacomb.Catacomb
	runner          *worker.Runner
	dbDeleter       coredatabase.DBDeleter
	deletionService ModelDatabaseDeletionService
	logger          logger.Logger
	clock           clock.Clock
}

// NewWorker creates a new instance of the model DB deleter worker.
func NewWorker(config Config) (worker.Worker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "model-db-deleter",
		IsFatal: func(err error) bool {
			return false
		},
		ShouldRestart: func(err error) bool {
			return true
		},
		Clock:  config.Clock,
		Logger: internalworker.WrapLogger(config.Logger),
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	w := &Worker{
		runner:          runner,
		dbDeleter:       config.DBDeleter,
		deletionService: config.DeletionService,
		logger:          config.Logger,
		clock:           config.Clock,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "model-db-deleter",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{w.runner},
	}); err != nil {
		return nil, err
	}

	return w, nil
}

// Kill stops the worker gracefully.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

// Report reports the internal state of the worker.
func (w *Worker) Report(ctx context.Context) map[string]any {
	return w.runner.Report(ctx)
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.deletionService.WatchModelDatabaseDeletions(ctx)
	if err != nil {
		return errors.Errorf("watching staged model database deletions: %w", err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Errorf("adding watcher to catacomb: %w", err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-watcher.Changes():
			// Get all of the staged deletions and handle them all at once.
			namespaces, err := w.deletionService.GetPendingModelDatabaseDeletions(ctx)
			if err != nil {
				return errors.Errorf("getting pending model database deletions: %w", err)
			}

			for _, namespace := range namespaces {
				if err := w.handlePendingDeletion(ctx, namespace); err != nil {
					return errors.Errorf("handling model database deletion %s: %w", namespace, err)
				}
			}
		}
	}
}

func (w *Worker) handlePendingDeletion(ctx context.Context, namespace string) error {
	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		return newDeletionWorker(namespace, w.deletionService, w.dbDeleter, w.logger), nil
	})
	if err != nil && !errors.Is(err, jujuerrors.AlreadyExists) {
		return errors.Errorf("starting worker for model database deletion %s: %w", namespace, err)
	}

	return nil
}

type deletionWorker struct {
	tomb tomb.Tomb

	namespace string
	service   ModelDatabaseDeletionService
	dbDeleter coredatabase.DBDeleter
	logger    logger.Logger
}

func newDeletionWorker(
	namespace string,
	service ModelDatabaseDeletionService,
	dbDeleter coredatabase.DBDeleter,
	logger logger.Logger,
) *deletionWorker {
	w := &deletionWorker{
		namespace: namespace,
		service:   service,
		dbDeleter: dbDeleter,
		logger:    logger,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the deletion worker gracefully.
func (w *deletionWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the deletion worker to finish.
func (w *deletionWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *deletionWorker) loop() error {
	ctx := w.tomb.Context(context.Background())

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		default:
			if err := w.deleteDatabase(ctx); err != nil {
				return errors.Capture(err)
			}

			return nil
		}
	}
}

func (w *deletionWorker) deleteDatabase(ctx context.Context) error {
	// Delete the model's dqlite database. A not-found database is already
	// gone, so treat it as success. Any other failure is returned so the
	// runner restarts this worker with backoff and the staged row survives to
	// be retried.
	if err := w.dbDeleter.DeleteDB(w.namespace); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		w.logger.Errorf(ctx,
			"deleting database for model %q: the model no longer resides on "+
				"this controller, but its database is still present: %v",
			w.namespace, err)
		return errors.Errorf("deleting database for model %q: %w", w.namespace, err)
	}

	// The database is gone; drop the staged deletion so the worker stops
	// retrying it.
	if err := w.service.RemoveModelDatabaseDeletion(ctx, w.namespace); err != nil {
		return errors.Errorf("removing staged deletion for model %q: %w", w.namespace, err)
	}

	return nil
}
