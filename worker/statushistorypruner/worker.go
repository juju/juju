// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

// HistoryPrunerParams specifies how history logs should be prunned.
type HistoryPrunerParams struct {
	// TODO(perrito666) We might want to have some sort of limitation of the collection size too.
	MaxLogsPerState int
	PruneInterval   time.Duration
}

const DefaultMaxLogsPerState = 100
const DefaultPruneInterval = 5 * time.Minute

// NewHistoryPrunerParams returns a HistoryPrunerParams initialized with default parameter.
func NewHistoryPrunerParams() *HistoryPrunerParams {
	return &HistoryPrunerParams{
		MaxLogsPerState: DefaultMaxLogsPerState,
		PruneInterval:   DefaultPruneInterval,
	}
}

type pruneHistoryFunc func(*state.State, int) error

type pruneWorker struct {
	st     *state.State
	params *HistoryPrunerParams
	pruner pruneHistoryFunc
}

// New returns a worker.Worker for history Pruner.
func New(st *state.State, params *HistoryPrunerParams) worker.Worker {
	w := &pruneWorker{
		st:     st,
		params: params,
		pruner: state.PruneStatusHistory,
	}
	return worker.NewPeriodicWorker(w.doPruning, w.params.PruneInterval, worker.NewTimer)
}

func (w *pruneWorker) doPruning(stop <-chan struct{}) error {
	err := w.pruner(w.st, w.params.MaxLogsPerState)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
