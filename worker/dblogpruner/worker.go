// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dblogpruner

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/worker.v1"
	"gopkg.in/tomb.v1"

	"github.com/juju/juju/state"
	jworker "github.com/juju/juju/worker"
)

var logger = loggo.GetLogger("juju.worker.dblogpruner")

// LogPruneParams specifies how logs should be pruned.
type LogPruneParams struct {
	PruneInterval time.Duration
}

const DefaultPruneInterval = 5 * time.Minute

// NewLogPruneParams returns a LogPruneParams initialised with default values.
func NewLogPruneParams() *LogPruneParams {
	return &LogPruneParams{
		PruneInterval: DefaultPruneInterval,
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
	return jworker.NewSimpleWorker(w.loop)
}

type pruneWorker struct {
	st     *state.State
	params *LogPruneParams
}

func (w *pruneWorker) loop(stopCh <-chan struct{}) error {

	controllerConfigWatcher := w.st.WatchControllerConfig()
	defer worker.Stop(controllerConfigWatcher)

	var (
		maxLogAge               time.Duration
		maxCollectionMB         int
		controllerConfigChanges = controllerConfigWatcher.Changes()
		// We will also get an initial event, but need to ensure that event is
		// received before doing any pruning.
		haveConfig = false
	)
	p := w.params

	for {
		select {
		case <-stopCh:
			return tomb.ErrDying
		case _, ok := <-controllerConfigChanges:
			if !ok {
				return errors.New("controller configuration watcher closed")
			}
			controllerConfig, err := w.st.ControllerConfig()
			if err != nil {
				return errors.Annotate(err, "cannot load controller configuration")
			}
			haveConfig = true
			newMaxAge := controllerConfig.MaxLogsAge()
			newMaxCollectionMB := controllerConfig.MaxLogSizeMB()
			if newMaxAge != maxLogAge || newMaxCollectionMB != maxCollectionMB {
				logger.Infof("log pruning config: max age: %v, max collection size %dM", newMaxAge, newMaxCollectionMB)
				maxLogAge = newMaxAge
				maxCollectionMB = newMaxCollectionMB
			}
			continue
		case <-time.After(p.PruneInterval):
			if !haveConfig {
				continue
			}
			// TODO(fwereade): 2016-03-17 lp:1558657
			minLogTime := time.Now().Add(-maxLogAge)
			err := state.PruneLogs(w.st, minLogTime, maxCollectionMB)
			if err != nil {
				return errors.Trace(err)
			}
		}
	}
}
