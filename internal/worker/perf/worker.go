// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/services"
	internalworker "github.com/juju/juju/internal/worker"
)

type perfWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner
	clock    clock.Clock
	logger   logger.Logger

	id int64

	modelUUID      model.UUID
	domainServices services.DomainServices
}

func newPerfWorker(
	modelUUID model.UUID,
	domainServices services.DomainServices,
	clock clock.Clock,
	logger logger.Logger,
) (*perfWorker, error) {
	w := &perfWorker{
		modelUUID:      modelUUID,
		clock:          clock,
		logger:         logger,
		domainServices: domainServices,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: clock,
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return true
			},
			RestartDelay: time.Second * 10,
			Logger:       internalworker.WrapLogger(logger),
		}),
	}

	if err := catacomb.Invoke(catacomb.Plan{
		Work: w.run,
		Site: &w.catacomb,
		Init: []worker.Worker{
			w.runner,
		},
	}); err != nil {
		return nil, err
	}

	return w, nil
}

// Wait blocks until the worker has finished.
func (w *perfWorker) Wait() error {
	return w.catacomb.Wait()
}

// Kill stops the worker.
func (w *perfWorker) Kill() {
	w.catacomb.Kill(nil)
}

func (w *perfWorker) run() error {
	ctx, cancel := context.WithCancel(w.catacomb.Context(context.Background()))
	defer cancel()

	controllerModelUUID, err := w.domainServices.Controller().ControllerModelUUID(ctx)
	if err != nil {
		return err
	}

	// Don't run the performance test on the controller model.
	if controllerModelUUID == w.modelUUID {
		w.logger.Infof(ctx, "%s: Controller model, not running performance test", w.modelUUID)
		return nil
	}

	w.logger.Debugf(ctx, "%s: Starting performance test", w.modelUUID)

	// Every 30 seconds, create a new step worker.
	timer := w.clock.NewTimer(time.Second * 30)
	for {
		select {
		case <-w.catacomb.Dying():
			w.logger.Debugf(ctx, "%s: Catacomb is dying", w.modelUUID)
			timer.Stop()
			return nil

		case <-timer.Chan():
			w.id++
			name := fmt.Sprintf("step-%d", w.id)

			w.logger.Infof(ctx, "%s: Creating worker step %s", w.modelUUID, name)

			if err := w.runner.StartWorker(name, func() (worker.Worker, error) {
				return newStepWorker(w.modelUUID, w.domainServices, w.id, w.clock, w.logger), nil
			}); err != nil {
				return err
			}

			// Stop the timer if we've reached 10 step workers.
			if w.id >= 10 {
				timer.Stop()
			} else {
				timer.Reset(time.Second * 30)
			}
		}
	}
}

type stepWorker struct {
	tomb tomb.Tomb

	modelUUID model.UUID
	services  services.DomainServices

	id     int64
	clock  clock.Clock
	logger logger.Logger
}

func newStepWorker(
	modelUUID model.UUID,
	services services.DomainServices,
	id int64,
	clock clock.Clock,
	logger logger.Logger,
) *stepWorker {
	w := &stepWorker{
		modelUUID: modelUUID,
		services:  services,
		id:        id,
		clock:     clock,
		logger:    logger,
	}
	w.tomb.Go(w.run)
	return w
}

// Wait blocks until the worker has finished.
func (w *stepWorker) Wait() error {
	return w.tomb.Wait()
}

// Kill stops the worker.
func (w *stepWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *stepWorker) run() error {
	ctx, cancel := context.WithCancel(w.tomb.Context(context.Background()))
	defer cancel()

	timer := w.clock.NewTimer(time.Second)
	for {
		select {
		case <-w.tomb.Dying():
			w.logger.Debugf(ctx, "%s: Tomb is dying %d", w.modelUUID, w.id)
			timer.Stop()
			return nil

		case <-timer.Chan():
			w.logger.Debugf(ctx, "%s: Step %d: starting", w.modelUUID, w.id)

			for i := 0; i < 20; i++ {
				if err := w.runStep(ctx, i); err != nil {
					w.logger.Errorf(ctx, "Failed to run step %d: %v", w.id, err)
					continue
				}
			}

			w.logger.Debugf(ctx, "%s: Step %d: finished", w.modelUUID, w.id)

			timer.Reset(time.Second)
		}
	}
}

func (w *stepWorker) runStep(ctx context.Context, step int) error {
	switch step % 6 {
	case 1:
		// Controller access.
		access := w.services.Access()
		_, err := access.GetAllUsers(ctx, true)
		return err
	case 2:
		// Controller access.
		modelDefaults := w.services.ModelDefaults()
		_, err := modelDefaults.ModelDefaults(ctx, w.modelUUID)
		return err
	case 3:
		// Controller access.
		model := w.services.Model()
		_, err := model.Model(ctx, w.modelUUID)
		return err
	case 4:
		// Model access.
		agent := w.services.Agent()
		_, err := agent.GetModelTargetAgentVersion(ctx)
		return err
	case 5:
		// Model access.
		modelInfo := w.services.ModelInfo()
		_, err := modelInfo.GetModelInfo(ctx)
		return err
	case 6:
		// Model access.
		machine := w.services.Machine()
		_, err := machine.AllMachineNames(ctx)
		return err
	default:
		// Controller access.
		controllerConfig := w.services.ControllerConfig()
		_, err := controllerConfig.ControllerConfig(ctx)
		return err
	}
}
