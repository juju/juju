// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import (
	"github.com/juju/juju/api"
	"github.com/juju/juju/worker"
)

func NewPruneWorker(api api.Connection, params *HistoryPrunerParams, t worker.NewTimerFunc, prune pruneFunc) worker.Worker {
	w := &pruneWorker{
		statusHistory: nil,
		params:        params,
		prune:         prune,
	}
	return worker.NewPeriodicWorker(w.doPruning, w.params.PruneInterval, t)
}
