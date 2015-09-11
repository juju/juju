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

// DefaultMaxLogsPerEntity is the default value for logs for each entity
// that should be kept at any given time.
const DefaultMaxLogsPerEntity = 100

// DefaultPruneInterval is the default interval that should be waited
// between prune calls.
const DefaultPruneInterval = 5 * time.Minute

// DefaultNewTimer is the timer constructor to be used when running
// on production.
var DefaultNewTimer = worker.NewTimer

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(int) error
}

// Config holds all necessary attributes to start a pruner worker.
type Config struct {
	Facade           Facade
	MaxLogsPerEntity int
	PruneInterval    time.Duration
	NewTimer         worker.NewTimerFunc
}

// New returns a worker.Worker for history Pruner.
func New(conf Config) worker.Worker {

	doPruning := func(stop <-chan struct{}) error {
		err := conf.Facade.Prune(conf.MaxLogsPerEntity)
		if err != nil {
			return errors.Trace(err)
		}
		return nil
	}

	return worker.NewPeriodicWorker(doPruning, conf.PruneInterval, conf.NewTimer)
}
