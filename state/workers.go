// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/lease"
)

const (
	txnLogWorker          = "txnlog"
	presenceWorker        = "presence"
	leadershipWorker      = "leadership"
	singularWorker        = "singular"
	allManagerWorker      = "allmanager"
	allModelManagerWorker = "allmodelmanager"
)

// workers runs the workers that a State instance requires.
// It wraps a Runner instance which restarts any of the
// workers when they fail.
type workers struct {
	state *State
	*worker.Runner
}

func newWorkers(st *State) (*workers, error) {
	ws := &workers{
		state: st,
		Runner: worker.NewRunner(worker.RunnerParams{
			// TODO add a Logger parameter to RunnerParams:
			// Logger: loggo.GetLogger(logger.Name() + ".workers"),
			IsFatal:      func(err error) bool { return err == jworker.ErrRestartAgent },
			RestartDelay: time.Second,
			Clock:        st.clock,
		}),
	}
	ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.New(st.getTxnLogCollection(), nil), nil
	})
	ws.StartWorker(presenceWorker, func() (worker.Worker, error) {
		return presence.NewWatcher(st.getPresenceCollection(), st.ModelTag()), nil
	})
	ws.StartWorker(leadershipWorker, func() (worker.Worker, error) {
		client, err := st.getLeadershipLeaseClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		manager, err := lease.NewManager(lease.ManagerConfig{
			Secretary: leadershipSecretary{},
			Client:    client,
			Clock:     ws.state.clock,
			MaxSleep:  time.Minute,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	})
	ws.StartWorker(singularWorker, func() (worker.Worker, error) {
		client, err := ws.state.getSingularLeaseClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		manager, err := lease.NewManager(lease.ManagerConfig{
			Secretary: singularSecretary{st.ModelUUID()},
			Client:    client,
			Clock:     st.clock,
			MaxSleep:  time.Minute,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	})
	return ws, nil
}

func (ws *workers) txnLogWatcher() *watcher.Watcher {
	w, err := ws.Worker(txnLogWorker, nil)
	if err != nil {
		return watcher.NewDead(errors.Trace(err))
	}
	return w.(*watcher.Watcher)
}

func (ws *workers) presenceWatcher() *presence.Watcher {
	w, err := ws.Worker(presenceWorker, nil)
	if err != nil {
		return presence.NewDeadWatcher(errors.Trace(err))
	}
	return w.(*presence.Watcher)
}

func (ws *workers) leadershipManager() *lease.Manager {
	w, err := ws.Worker(leadershipWorker, nil)
	if err != nil {
		return lease.NewDeadManager(errors.Trace(err))
	}
	return w.(*lease.Manager)
}

func (ws *workers) singularManager() *lease.Manager {
	w, err := ws.Worker(singularWorker, nil)
	if err != nil {
		return lease.NewDeadManager(errors.Trace(err))
	}
	return w.(*lease.Manager)
}

func (ws *workers) allManager(params WatchParams) *storeManager {
	w, err := ws.Worker(allManagerWorker, nil)
	if err == nil {
		return w.(*storeManager)
	}
	if errors.Cause(err) != worker.ErrNotFound {
		return newDeadStoreManager(errors.Trace(err))
	}
	// Note that StartWorker is idempotent if there's a race.
	ws.StartWorker(allManagerWorker, func() (worker.Worker, error) {
		return newStoreManager(newAllWatcherStateBacking(ws.state, params)), nil
	})
	return ws.allManager(params)
}

func (ws *workers) allModelManager(pool *StatePool) *storeManager {
	w, err := ws.Worker(allModelManagerWorker, nil)
	if err == nil {
		return w.(*storeManager)
	}
	if errors.Cause(err) != worker.ErrNotFound {
		return newDeadStoreManager(errors.Trace(err))
	}
	ws.StartWorker(allModelManagerWorker, func() (worker.Worker, error) {
		return newStoreManager(NewAllModelWatcherStateBacking(ws.state, pool)), nil
	})
	return ws.allModelManager(pool)
}
