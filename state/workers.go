// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"github.com/juju/worker/v2"

	"github.com/juju/juju/state/watcher"
	jworker "github.com/juju/juju/worker"
)

const (
	txnLogWorker = "txnlog"
	txnLogPruner = "txnlog-pruner"
)

var txnLogPruneInterval = 10 * time.Minute

// workers runs the workers that a State instance requires.
// It wraps a Runner instance which restarts any of the
// workers when they fail.
type workers struct {
	state *State
	//	model *Model
	*worker.Runner

	hub *pubsub.SimpleHub
}

func newWorkers(st *State, hub *pubsub.SimpleHub) (*workers, error) {
	if hub == nil {
		return nil, errors.NotValidf("missing hub")
	}
	ws := &workers{
		state: st,
		hub:   hub,
		Runner: worker.NewRunner(worker.RunnerParams{
			// TODO add a Logger parameter to RunnerParams:
			// Logger: loggo.GetLogger(logger.Name() + ".workers"),
			IsFatal:      func(err error) bool { return err == jworker.ErrRestartAgent },
			RestartDelay: time.Second,
			Clock:        st.clock(),
		}),
	}
	_ = ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.NewHubWatcher(watcher.HubWatcherConfig{
			Hub:       hub,
			Clock:     st.clock(),
			ModelUUID: st.modelUUID(),
			Logger:    loggo.GetLogger("juju.state.watcher"),
		})
	})
	// The controller also needs to prune the txn log collection.
	if st.IsController() {
		_ = ws.StartWorker(txnLogPruner, func() (worker.Worker, error) {
			return jworker.NewPeriodicWorker(
				ws.txnLogPruner, txnLogPruneInterval, jworker.NewTimer,
			), nil
		})
	}
	return ws, nil
}

func (ws *workers) txnLogWatcher() watcher.BaseWatcher {
	w, err := ws.Worker(txnLogWorker, nil)
	if err != nil {
		return watcher.NewDead(errors.Trace(err))
	}
	return w.(watcher.BaseWatcher)
}

func (ws *workers) txnLogPruner(stop <-chan struct{}) error {
	cfg, err := ws.state.ControllerConfig()
	if err != nil {
		return errors.Trace(err)
	}
	coll, closer := ws.state.db().GetRawCollection(txnLogC)
	defer closer()

	txnLogSizeMB := cfg.MaxTxnLogSizeMB()
	err = pruneCollection(stop, ws.state, 0, txnLogSizeMB, coll, "", nil, "")
	return errors.Trace(err)
}
