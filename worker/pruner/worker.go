// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package pruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.pruner")

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(time.Duration, int) error
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

// Worker prunes status history records at regular intervals.
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

// returns the prune worker's catacomb
func (w *PrunerWorker) Catacomb() *catacomb.Catacomb {
	return &w.catacomb
}

// return the prune worker's config
func (w *PrunerWorker) Config() *Config {
	return &w.config
}

// body of generic pruner loop
func (w *PrunerWorker) Work(getPrunerConfig func(*config.Config) (time.Duration, uint)) error {
	modelConfigWatcher, err := w.config.Facade.WatchForModelConfigChanges()
	if err != nil {
		return errors.Trace(err)
	}
	err = w.catacomb.Add(modelConfigWatcher)
	if err != nil {
		return errors.Trace(err)
	}

	var (
		maxAge             time.Duration
		maxCollectionMB    uint
		modelConfigChanges = modelConfigWatcher.Changes()
		// We will also get an initial event, but need to ensure that event is
		// received before doing any pruning.
	)

	var timer clock.Timer
	var timerCh <-chan time.Time
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-modelConfigChanges:
			if !ok {
				return errors.New("model configuration watcher closed")
			}
			modelConfig, err := w.config.Facade.ModelConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load model configuration")
			}

			newMaxAge, newMaxCollectionMB := getPrunerConfig(modelConfig)

			if newMaxAge != maxAge || newMaxCollectionMB != maxCollectionMB {
				logger.Infof("status history config: max age: %v, max collection size %dM for %s (%s)",
					newMaxAge, newMaxCollectionMB, modelConfig.Name(), modelConfig.UUID())
				maxAge = newMaxAge
				maxCollectionMB = newMaxCollectionMB
			}
			if timer == nil {
				timer = w.config.Clock.NewTimer(w.config.PruneInterval)
				timerCh = timer.Chan()
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
