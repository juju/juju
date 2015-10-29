// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/worker"
)

// HistoryPrunerParams specifies how history logs should be prunned.
type HistoryPrunerParams struct {
	// TODO(perrito666) We might want to have some sort of limitation of the collection size too.
	MaxLogsPerEntity int
	PruneInterval    time.Duration
}

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(int) error
}

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade           Facade
	MaxLogsPerEntity uint
	PruneInterval    time.Duration
	NewTimer         worker.NewTimerFunc
}

// Validate will err unless basic requirements for a valid
// config are met.
func (c *Config) Validate() error {
	if c.Facade == nil {
		return errors.New("missing Facade")
	}
	if c.NewTimer == nil {
		return errors.New("missing Timer")
	}
	return nil
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) (worker.Worker, error) {
	if err := conf.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	doPruning := func(stop <-chan struct{}) error {
		err := conf.Facade.Prune(int(conf.MaxLogsPerEntity))
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	return worker.NewPeriodicWorker(doPruning, conf.PruneInterval, conf.NewTimer), nil
}
