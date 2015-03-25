// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner

import (
	"time"

	"github.com/juju/errors"
	"launchpad.net/tomb"

	"github.com/juju/juju/state"
	"github.com/juju/juju/worker"
)

// LogPruneParams specifies how logs should be pruned.
type LogPruneParams struct {
	MaxLogAge       time.Duration
	MaxCollectionMB int
	PruneInterval   time.Duration
}

const DefaultMaxLogAge = 3 * 24 * time.Hour // 3 days
const DefaultMaxCollectionMB = 4 * 1024     // 4 GB
const DefaultPruneInterval = 5 * time.Minute

// NewLogPruneParams returns a LogPruneParams initialised with default
// values.
func NewLogPruneParams() *LogPruneParams {
	return &LogPruneParams{
		MaxLogAge:       DefaultMaxLogAge,
		MaxCollectionMB: DefaultMaxCollectionMB,
		PruneInterval:   DefaultPruneInterval,
	}
}

// New returns a worker which periodically wakes up to remove old log
// entries stored in MongoDB. This worker is intended to run just
// once, on the MongoDB master.
func New(st *state.State, params *LogPruneParams) worker.Worker {
	w := &pruneWorker{
		st:     st,
		params: params,
	}
	return worker.NewSimpleWorker(w.loop)
}

type pruneWorker struct {
	st     *state.State
	params *LogPruneParams
}

func (w *pruneWorker) loop(stopCh <-chan struct{}) error {
	p := w.params
	for {
		select {
		case <-stopCh:
			return tomb.ErrDying
		case <-time.After(p.PruneInterval):
			minLogTime := time.Now().Add(-p.MaxLogAge)
			err := state.PruneLogs(w.st, minLogTime, p.MaxCollectionMB)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}
