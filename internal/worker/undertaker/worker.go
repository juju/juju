// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/catacomb"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	coretrace "github.com/juju/juju/core/trace"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
	internalworker "github.com/juju/juju/internal/worker"
)

// Config holds the configuration for the undertaker worker.
type Config struct {
	DBDeleter              coredatabase.DBDeleter
	ControllerModelService ControllerModelService
	RemovalServiceGetter   RemovalServiceGetter
	Logger                 logger.Logger
	Clock                  clock.Clock
	Tracer                 coretrace.Tracer
}

// Worker is the undertaker worker that manages the lifecycle of resources.
type Worker struct {
	catacomb               catacomb.Catacomb
	runner                 *worker.Runner
	dbDeleter              coredatabase.DBDeleter
	controllerModelService ControllerModelService
	removalServiceGetter   RemovalServiceGetter
	logger                 logger.Logger
	clock                  clock.Clock
	tracer                 coretrace.Tracer
}

// NewWorker creates a new instance of the undertaker worker.
func NewWorker(config Config) (worker.Worker, error) {
	runner, err := worker.NewRunner(worker.RunnerParams{
		Name: "undertaker",
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
		runner:                 runner,
		dbDeleter:              config.DBDeleter,
		controllerModelService: config.ControllerModelService,
		removalServiceGetter:   config.RemovalServiceGetter,
		logger:                 config.Logger,
		clock:                  config.Clock,
		tracer:                 config.Tracer,
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Name: "undertaker",
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

	modelWatcher, err := w.controllerModelService.WatchModels(ctx)
	if err != nil {
		return errors.Errorf("watching activated models: %w", err)
	}
	if err := w.catacomb.Add(modelWatcher); err != nil {
		return errors.Errorf("adding model watcher to catacomb: %w", err)
	}

	deletionWatcher, err := w.controllerModelService.WatchModelMigrationDeletions(ctx)
	if err != nil {
		return errors.Errorf("watching staged model database deletions: %w", err)
	}
	if err := w.catacomb.Add(deletionWatcher); err != nil {
		return errors.Errorf("adding deletion watcher to catacomb: %w", err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case <-modelWatcher.Changes():
			// Get all of the dead models from the controller model service
			// and handle them all at once.
			models, err := w.controllerModelService.GetDeadModels(ctx)
			if err != nil {
				return errors.Errorf("getting dead models: %w", err)
			}

			// Attempt to handle dead models first, this is graceful death.
			for _, mUUID := range models {
				if err := w.handleDeadModel(ctx, mUUID); err != nil {
					return errors.Errorf("handling dead model %s: %w", mUUID, err)
				}
			}

		case <-deletionWatcher.Changes():
			// Get all of the staged database deletions and handle them all at
			// once. These are models that have already been purged from this
			// controller (currently by source-side migration REAP) but whose
			// dqlite database still exists.
			namespaces, err := w.controllerModelService.GetPendingModelDatabaseDeletions(ctx)
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

func (w *Worker) handleDeadModel(ctx context.Context, mUUID model.UUID) error {
	err := w.runner.StartWorker(ctx, mUUID.String(), func(ctx context.Context) (worker.Worker, error) {
		removalService, err := w.removalServiceGetter.GetRemovalService(ctx, mUUID)
		if err != nil {
			return nil, errors.Errorf("getting removal service for model %s: %w", mUUID, err)
		}

		return newModelWorker(mUUID, removalService, w.dbDeleter, w.tracer), nil
	})
	if err != nil && !errors.Is(err, jujuerrors.AlreadyExists) {
		return errors.Errorf("starting worker for model %s: %w", mUUID, err)
	}

	return nil
}

func (w *Worker) handlePendingDeletion(ctx context.Context, namespace string) error {
	// The namespace is the model UUID, so reuse it as the runner key. A staged
	// database deletion and a dead-model worker for the same model then share a
	// key, and the runner guarantees only one of them runs at a time. This is
	// impossible today, but it means it can never happen in the future either.
	err := w.runner.StartWorker(ctx, namespace, func(ctx context.Context) (worker.Worker, error) {
		return newDBDeletionWorker(namespace, w.controllerModelService, w.dbDeleter, w.logger, w.tracer), nil
	})
	if err != nil && !errors.Is(err, jujuerrors.AlreadyExists) {
		return errors.Errorf("starting worker for model database deletion %s: %w", namespace, err)
	}

	return nil
}

type modelWorker struct {
	tomb tomb.Tomb

	modelUUID      model.UUID
	removalService RemovalService
	dbDeleter      coredatabase.DBDeleter
	tracer         coretrace.Tracer
}

func newModelWorker(
	modelUUID model.UUID,
	removalService RemovalService,
	dbDeleter coredatabase.DBDeleter,
	tracer coretrace.Tracer,
) *modelWorker {
	w := &modelWorker{
		modelUUID:      modelUUID,
		removalService: removalService,
		dbDeleter:      dbDeleter,
		tracer:         tracer,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the model worker gracefully.
func (w *modelWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the model worker to finish.
func (w *modelWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *modelWorker) loop() error {
	ctx := w.tomb.Context(context.Background())

	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying

		default:
			if err := w.deleteModel(ctx); err != nil {
				return errors.Capture(err)
			}

			return nil
		}
	}
}

func (w *modelWorker) deleteModel(ctx context.Context) (err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, w.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("undertaker.model.uuid", w.modelUUID.String())),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	if err := w.removalService.DeleteModel(ctx); err != nil && !errors.IsOneOf(err,
		coredatabase.ErrDBNotFound,
		modelerrors.NotFound,
	) {
		return errors.Errorf("deleting model: %w", err)
	}

	if err := w.dbDeleter.DeleteDB(w.modelUUID.String()); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return errors.Errorf("deleting database %s: %w", w.modelUUID, err)
	}

	return nil
}

// dbDeletionWorker deletes the dqlite database of a single model that has been
// purged from this controller, then drops the staged deletion row. It is
// restarted with backoff by the runner until the database is gone and the
// staged row removed.
type dbDeletionWorker struct {
	tomb tomb.Tomb

	namespace string
	service   ControllerModelService
	dbDeleter coredatabase.DBDeleter
	logger    logger.Logger
	tracer    coretrace.Tracer
}

func newDBDeletionWorker(
	namespace string,
	service ControllerModelService,
	dbDeleter coredatabase.DBDeleter,
	logger logger.Logger,
	tracer coretrace.Tracer,
) *dbDeletionWorker {
	w := &dbDeletionWorker{
		namespace: namespace,
		service:   service,
		dbDeleter: dbDeleter,
		logger:    logger,
		tracer:    tracer,
	}

	w.tomb.Go(w.loop)

	return w
}

// Kill stops the deletion worker gracefully.
func (w *dbDeletionWorker) Kill() {
	w.tomb.Kill(nil)
}

// Wait waits for the deletion worker to finish.
func (w *dbDeletionWorker) Wait() error {
	return w.tomb.Wait()
}

func (w *dbDeletionWorker) loop() error {
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

func (w *dbDeletionWorker) deleteDatabase(ctx context.Context) (err error) {
	ctx, span := coretrace.Start(coretrace.WithTracer(ctx, w.tracer), coretrace.NameFromFunc(),
		coretrace.WithAttributes(coretrace.StringAttr("undertaker.database.namespace", w.namespace)),
	)
	defer func() {
		span.RecordError(err)
		span.End()
	}()

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
