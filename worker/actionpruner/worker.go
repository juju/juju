// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/worker/catacomb"
	"github.com/juju/juju/worker/pruner"
)

// Worker prunes status history records at regular intervals.
type Worker struct {
	pruner.PrunerWorker
}

func NewFacade(caller base.APICaller) pruner.Facade {
	return action.NewFacade(caller)
}

func (w *Worker) loop() error {
	return w.Work(func(config *config.Config) (time.Duration, uint) {
		return config.MaxActionResultsAge(), config.MaxActionResultsSizeMB()
	})
}

// New creates a new action pruner worker
func New(conf pruner.Config) (worker.Worker, error) {
	if err := conf.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &Worker{
		pruner.New(conf),
	}

	err := catacomb.Invoke(catacomb.Plan{
		Site: w.Catacomb(),
		Work: w.loop,
	})

	return w, errors.Trace(err)
}
