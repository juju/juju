// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package perf

import (
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/state"
)

type Logger interface {
	Debugf(string, ...interface{})
	Infof(string, ...interface{})
	Errorf(string, ...interface{})
}

type perfWorker struct {
	catacomb catacomb.Catacomb
	runner   *worker.Runner
	clock    clock.Clock
	logger   Logger

	id int64

	modelUUID   string
	systemState *state.State
	state       *state.State
}

func newPerfWorker(modelUUID string, systemState, state *state.State, clock clock.Clock, logger Logger) (*perfWorker, error) {
	w := &perfWorker{
		modelUUID:   modelUUID,
		clock:       clock,
		logger:      logger,
		systemState: systemState,
		state:       state,
		runner: worker.NewRunner(worker.RunnerParams{
			Clock: clock,
			IsFatal: func(err error) bool {
				return false
			},
			ShouldRestart: func(err error) bool {
				return true
			},
			RestartDelay: time.Second * 10,
			Logger:       logger,
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
	controllerModelUUID := w.systemState.ControllerModelUUID()

	// Don't run the performance test on the controller model.
	if controllerModelUUID == w.modelUUID {
		w.logger.Infof("%s: Controller model, not running performance test", w.modelUUID)
		return nil
	}

	w.logger.Debugf("%s: Starting performance test", w.modelUUID)

	// Every 30 seconds, create a new step worker.
	timer := w.clock.NewTimer(time.Second * 30)
	for {
		select {
		case <-w.catacomb.Dying():
			w.logger.Debugf("%s: Catacomb is dying", w.modelUUID)
			timer.Stop()
			return nil

		case <-timer.Chan():
			w.id++
			name := fmt.Sprintf("step-%d", w.id)

			w.logger.Infof("%s: Creating worker step %s", w.modelUUID, name)

			if err := w.runner.StartWorker(name, func() (worker.Worker, error) {
				return newStepWorker(w.modelUUID, w.id, w.clock, w.logger, w.runStep), nil
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

func (w *perfWorker) runStep(step int) error {
	switch step % 6 {
	case 1:
		// Controller access.
		_, err := w.systemState.AllUsers(true)
		return err
	case 2:
		// Controller access.
		m, err := w.systemState.Model()
		if err != nil {
			return err
		}
		_, err = w.systemState.ModelConfigDefaultValues(m.CloudName())
		return err
	case 3:
		// Controller access.
		_, err := w.systemState.Model()
		return err
	case 4:
		// Model access.
		m, err := w.state.Model()
		if err != nil {
			return err
		}
		_, err = m.ModelConfig()
		return err
	case 5:
		// Model access.
		m, err := w.state.Model()
		if err != nil {
			return err
		}
		return m.Refresh()
	case 6:
		// Model access.
		_, err := w.state.AllMachines()
		return err
	default:
		// Controller access.
		_, err := w.systemState.ControllerConfig()
		return err
	}
}

type stepWorker struct {
	tomb tomb.Tomb

	modelUUID string

	id     int64
	clock  clock.Clock
	logger Logger

	step func(int) error
}

func newStepWorker(modelUUID string, id int64, clock clock.Clock, logger Logger, fn func(int) error) *stepWorker {
	w := &stepWorker{
		modelUUID: modelUUID,
		id:        id,
		clock:     clock,
		logger:    logger,
		step:      fn,
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
	timer := w.clock.NewTimer(time.Second)
	for {
		select {
		case <-w.tomb.Dying():
			w.logger.Debugf("%s: Tomb is dying %d", w.modelUUID, w.id)
			timer.Stop()
			return nil

		case <-timer.Chan():
			w.logger.Debugf("%s: Step %d: starting", w.modelUUID, w.id)

			for i := 0; i < 20; i++ {
				if err := w.step(i); err != nil {
					w.logger.Errorf("Failed to run step %d: %v", w.id, err)
					continue
				}
			}

			w.logger.Debugf("%s: Step %d: finished", w.modelUUID, w.id)

			timer.Reset(time.Second)
		}
	}
}
