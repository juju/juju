// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/environs/config"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
var logger interface{}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/mocks_facade.go github.com/juju/juju/worker/pruner Facade

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(time.Duration, int) error
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
	WatchForControllerConfigChanges() (watcher.NotifyWatcher, error)
	ControllerConfig() (controller.Config, error)
}

// PrunerWorker prunes status history or action records at regular intervals.
type PrunerWorker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is defined on worker.Worker.
func (w *PrunerWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *PrunerWorker) Wait() error {
	return w.catacomb.Wait()
}

// Catacomb returns the prune worker's catacomb.
func (w *PrunerWorker) Catacomb() *catacomb.Catacomb {
	return &w.catacomb
}

// Config return the prune worker's config.
func (w *PrunerWorker) Config() *Config {
	return &w.config
}

// Work is the main body of generic pruner loop.
func (w *PrunerWorker) Work(getPrunerConfig func(controller.Config, *config.Config) (time.Duration, uint)) error {
	var (
		maxAge                  time.Duration
		maxCollectionMB         uint
		controllerConfigChanges watcher.NotifyChannel
		modelConfigChanges      watcher.NotifyChannel
		// We will also get an initial event, but need to ensure that event is
		// received before doing any pruning.
	)

	modelConfigWatcher, err := w.config.Facade.WatchForModelConfigChanges()
	if err != nil && !errors.IsNotSupported(err) {
		return errors.Trace(err)
	}
	if err == nil {
		err = w.catacomb.Add(modelConfigWatcher)
		if err != nil {
			return errors.Trace(err)
		}
		modelConfigChanges = modelConfigWatcher.Changes()
	}
	controllerConfigWatcher, err := w.config.Facade.WatchForControllerConfigChanges()
	if err != nil && !errors.IsNotSupported(err) {
		return errors.Trace(err)
	}
	if err == nil {
		err = w.catacomb.Add(controllerConfigWatcher)
		if err != nil {
			return errors.Trace(err)
		}
		controllerConfigChanges = controllerConfigWatcher.Changes()
	}

	var timer clock.Timer
	var timerCh <-chan time.Time

	updateWorkerConfig := func() error {
		var (
			err              error
			controllerConfig controller.Config
			modelConfig      *config.Config
		)
		if controllerConfigWatcher != nil {
			controllerConfig, err = w.config.Facade.ControllerConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load controller configuration")
			}
		}
		if modelConfigWatcher != nil {
			modelConfig, err = w.config.Facade.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}
		}

		newMaxAge, newMaxCollectionMB := getPrunerConfig(controllerConfig, modelConfig)

		if newMaxAge != maxAge || newMaxCollectionMB != maxCollectionMB {
			if modelConfig != nil {
				w.config.Logger.Infof("pruner config: max age: %v, max collection size %dM for %s (%s)",
					newMaxAge, newMaxCollectionMB, modelConfig.Name(), modelConfig.UUID())
			} else {
				w.config.Logger.Infof("pruner config: max age: %v, max collection size %dM",
					newMaxAge, newMaxCollectionMB)
			}
			maxAge = newMaxAge
			maxCollectionMB = newMaxCollectionMB
		}
		if timer == nil {
			timer = w.config.Clock.NewTimer(w.config.PruneInterval)
			timerCh = timer.Chan()
		}
		return nil
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-controllerConfigChanges:
			if !ok {
				return errors.New("controller configuration watcher closed")
			}
			if err := updateWorkerConfig(); err != nil {
				return errors.Trace(err)
			}
		case _, ok := <-modelConfigChanges:
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			if err := updateWorkerConfig(); err != nil {
				return errors.Trace(err)
			}
		case <-timerCh:
			err := w.config.Facade.Prune(maxAge, int(maxCollectionMB))
			if err != nil {
				return errors.Trace(err)
			}
			timer.Reset(w.config.PruneInterval)
		}
	}
}
