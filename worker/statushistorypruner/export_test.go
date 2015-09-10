// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistorypruner

import "github.com/juju/juju/worker"

func NewPruneWorkerWithPrune(f Facade, params *HistoryPrunerParams, t worker.NewTimerFunc, prune pruneFunc) worker.Worker {
	w := &pruner{
		statusHistory: f,
		params:        params,
		prune:         prune,
	}
	return worker.NewPeriodicWorker(w.doPruning, w.params.PruneInterval, t)
}

func NewPruneWorker(f Facade, params *HistoryPrunerParams, t worker.NewTimerFunc) worker.Worker {
	w := newPruner(f, params)
	return worker.NewPeriodicWorker(w.doPruning, w.params.PruneInterval, t)
}
