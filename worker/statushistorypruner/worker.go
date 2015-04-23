// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"time"

	"github.com/juju/errors"
	"launchpad.net/tomb"

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

type pruneWorker struct {
	st     *state.State
	params *HistoryPrunerParams
}

func New(st *state.State, params *HistoryPrunerParams) worker.Worker {
	w := &pruneWorker{
		st:     st,
		params: params,
	}
	return worker.NewSimpleWorker(w.loop)
}

func (w *pruneWorker) loop(stopCh <-chan struct{}) error {
	p := w.params
	for {
		select {
		case <-stopCh:
			return tomb.ErrDying
		case <-time.After(p.PruneInterval):
			err := state.PruneStatusHistory(w.st, p.MaxLogsPerState)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}
