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
	MaxLogsPerState int
	PruneInterval   time.Duration
}

// DefaultMaxLogsPerState is the default value for logs for each entity
// that should be kept at any given time.
const DefaultMaxLogsPerState = 100

// DefaultPruneInterval is the default interval that should be waited
// between prune calls.
const DefaultPruneInterval = 5 * time.Minute

// NewHistoryPrunerParams returns a HistoryPrunerParams initialized with default parameter.
func NewHistoryPrunerParams() *HistoryPrunerParams {
	return &HistoryPrunerParams{
		MaxLogsPerState: DefaultMaxLogsPerState,
		PruneInterval:   DefaultPruneInterval,
	}
}

// Facade represents an API that implements status history pruning.
type Facade interface {
	Prune(int) error
}

type pruneFunc func(int) error

type pruner struct {
	statusHistory Facade
	params        *HistoryPrunerParams
	prune         pruneFunc
}

// newPruner is only to simplify testing of pruner without periodic worker.
func newPruner(facade Facade, params *HistoryPrunerParams) *pruner {
	prune := func(i int) error { return facade.Prune(i) }
	return &pruner{
		statusHistory: facade,
		params:        params,
		prune:         prune,
	}
}

// New returns a worker.Worker for history Pruner.
func New(facade Facade, params *HistoryPrunerParams) worker.Worker {
	w := newPruner(facade, params)
	return worker.NewPeriodicWorker(w.doPruning, w.params.PruneInterval, worker.NewTimer)
}

func (w *pruner) doPruning(stop <-chan struct{}) error {
	err := w.prune(w.params.MaxLogsPerState)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
