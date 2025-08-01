// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/clock"
	jujuerrors "github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
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
func (w *Worker) Report() map[string]any {
	return w.runner.Report()
}

func (w *Worker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.controllerModelService.WatchActivatedModels(ctx)
	if err != nil {
		return errors.Errorf("watching activated models: %w", err)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Errorf("adding watcher to catacomb: %w", err)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case models := <-watcher.Changes():
			deadModels, notFoundModels, err := w.filterDeadModels(ctx, models)
			if err != nil {
				return errors.Errorf("filtering dead models: %w", err)
			}

			// Attempt to handle dead models first, this is graceful death.
			for _, mUUID := range deadModels {
				if err := w.handleDeadModel(ctx, mUUID); err != nil {
					return errors.Errorf("handling dead model %s: %w", mUUID, err)
				}
			}

			// Not found models, are models which we've still got a reference
			// to, but they no longer exist in the controller.
			for _, mUUID := range notFoundModels {
				if err := w.handleNotFoundModel(ctx, mUUID); err != nil {
					return errors.Errorf("handling not found model %s: %w", mUUID, err)
				}
			}
		}
	}
}

func (w *Worker) filterDeadModels(ctx context.Context, uuids []string) (dead, notFound []model.UUID, err error) {
	for _, uuid := range uuids {
		mUUID := model.UUID(uuid)
		mLife, err := w.controllerModelService.GetModelLife(ctx, mUUID)
		if errors.Is(err, modelerrors.NotFound) {
			notFound = append(notFound, mUUID)
			continue
		} else if err != nil {
			return nil, nil, errors.Errorf("getting model life for %s: %w", uuid, err)
		}

		if mLife != life.Dead {
			continue
		}

		dead = append(dead, mUUID)
	}
	return dead, notFound, nil
}

func (w *Worker) handleDeadModel(ctx context.Context, mUUID model.UUID) error {
	err := w.runner.StartWorker(ctx, mUUID.String(), func(ctx context.Context) (worker.Worker, error) {
		removalService, err := w.removalServiceGetter.GetRemovalService(ctx, mUUID)
		if err != nil {
			return nil, errors.Errorf("getting removal service for model %s: %w", mUUID, err)
		}

		return newModelWorker(mUUID, removalService, w.dbDeleter), nil
	})
	if err != nil && !errors.Is(err, jujuerrors.AlreadyExists) {
		return errors.Errorf("starting worker for model %s: %w", mUUID, err)
	}

	return nil
}

func (w *Worker) handleNotFoundModel(ctx context.Context, mUUID model.UUID) error {
	if err := w.runner.StopAndRemoveWorker(mUUID.String(), ctx.Done()); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return errors.Errorf("stopping and removing worker for model %s: %w", mUUID, err)
	}

	// Brute force the removal of the model from the controller.
	// We can guarantee that the model is not active, so we can safely delete it.
	if err := w.dbDeleter.DeleteDB(mUUID.String()); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		// Log the error but do not return it, as this will crash the
		// worker and prevent further processing of other models. Manual
		// intervention may be required to clean up the database.
		w.logger.Errorf(ctx, "deleting database for model %s: %v", mUUID, err)
	}

	return nil
}

type modelWorker struct {
	tomb tomb.Tomb

	modelUUID      model.UUID
	removalService RemovalService
	dbDeleter      coredatabase.DBDeleter
}

func newModelWorker(modelUUID model.UUID, removalService RemovalService, dbDeleter coredatabase.DBDeleter) *modelWorker {
	w := &modelWorker{
		modelUUID:      modelUUID,
		removalService: removalService,
		dbDeleter:      dbDeleter,
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

func (w *modelWorker) deleteModel(ctx context.Context) error {
	if err := w.removalService.DeleteModel(ctx); err != nil && !errors.Is(err, modelerrors.NotFound) {
		return errors.Errorf("deleting model: %w", err)
	}

	if err := w.dbDeleter.DeleteDB(w.modelUUID.String()); err != nil && !errors.Is(err, jujuerrors.NotFound) {
		return errors.Errorf("deleting database %s: %w", w.modelUUID, err)
	}

	return nil
}
