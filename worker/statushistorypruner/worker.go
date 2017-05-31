// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/clock"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/statushistory"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

var logger = loggo.GetLogger("juju.worker.statushistorypruner")

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(time.Duration, int) error
	WatchForModelConfigChanges() (watcher.NotifyWatcher, error)
	ModelConfig() (*config.Config, error)
}

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade        Facade
	PruneInterval time.Duration
	Clock         clock.Clock
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.Facade == nil {
		return errors.New("missing Facade")
	}
	if c.Clock == nil {
		return errors.New("missing Clock")
	}
	return nil
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) (worker.Worker, error) {
	if err := conf.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		config: conf,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, errors.Trace(err)
}

// NewFacade returns a new status history facade.
func NewFacade(caller base.APICaller) Facade {
	return statushistory.NewFacade(caller)
}

// Worker prunes status history records at regular intervals.
type Worker struct {
	catacomb catacomb.Catacomb
	config   Config
}

// Kill is defined on worker.Worker.
func (w *Worker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker.
func (w *Worker) Wait() error {
	return w.catacomb.Wait()
}

func (w *Worker) loop() error {

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
			newMaxAge := modelConfig.MaxStatusHistoryAge()
			newMaxCollectionMB := modelConfig.MaxStatusHistorySizeMB()
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
