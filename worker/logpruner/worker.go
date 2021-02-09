// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logpruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/logpruner"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/worker/pruner"
)

// logger is here to stop the desire of creating a package level logger.
// Don't do this, instead pass one through as config to the worker.
var logger interface{}

// Worker prunes status history records at regular intervals.
type Worker struct {
	pruner.PrunerWorker
}

// NewClient returns a new log pruner facade.
func NewClient(caller base.APICaller) pruner.Facade {
	return logpruner.NewClient(caller)
}

func (w *Worker) loop() error {
	return w.Work(func(config controller.Config, _ *config.Config) (time.Duration, uint) {
		return 0, uint(config.ModelLogsSizeMB())
	})
}

// New creates a new log pruner worker
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
