// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"

	corelease "github.com/juju/juju/core/lease"
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
	pingBatcherWorker     = "pingbatcher"
)

// workers runs the workers that a State instance requires.
// It wraps a Runner instance which restarts any of the
// workers when they fail.
type workers struct {
	state *State
	*worker.Runner
}

const pingFlushInterval = time.Second

func newWorkers(st *State, hub *pubsub.SimpleHub) (*workers, error) {
	ws := &workers{
		state: st,
		Runner: worker.NewRunner(worker.RunnerParams{
			// TODO add a Logger parameter to RunnerParams:
			// Logger: loggo.GetLogger(logger.Name() + ".workers"),
			IsFatal:      func(err error) bool { return err == jworker.ErrRestartAgent },
			RestartDelay: time.Second,
			Clock:        st.clock(),
		}),
	}
	if hub == nil {
		if err := ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
			return watcher.New(st.getTxnLogCollection()), nil
		}); err != nil {
			return nil, errors.Trace(err)
		}
	} else {
		if err := ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
			return watcher.NewHubWatcher(hub, loggo.GetLogger("juju.state.watcher")), nil
		}); err != nil {
			return nil, errors.Trace(err)
		}
	}
	if err := ws.StartWorker(presenceWorker, func() (worker.Worker, error) {
		return presence.NewWatcher(st.getPresenceCollection(), st.modelTag), nil
	}); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ws.StartWorker(pingBatcherWorker, func() (worker.Worker, error) {
		return presence.NewPingBatcher(st.getPresenceCollection(), pingFlushInterval), nil
	}); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ws.StartWorker(leadershipWorker, func() (worker.Worker, error) {
		manager, err := st.newLeaseManager(st.getLeadershipLeaseClient, leadershipSecretary{}, st.ModelUUID())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	}); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ws.StartWorker(singularWorker, func() (worker.Worker, error) {
		manager, err := st.newLeaseManager(st.getSingularLeaseClient,
			singularSecretary{
				controllerUUID: st.ControllerUUID(),
				modelUUID:      st.ModelUUID(),
			},
			st.ControllerUUID(),
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	}); err != nil {
		return nil, errors.Trace(err)
	}
	return ws, nil
}

func (st *State) newLeaseManager(
	getClient func() (corelease.Client, error),
	secretary lease.Secretary,
	entityUUID string,
) (worker.Worker, error) {
	client, err := getClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	manager, err := lease.NewManager(lease.ManagerConfig{
		Secretary:  secretary,
		Client:     client,
		Clock:      st.clock(),
		MaxSleep:   time.Minute,
		EntityUUID: entityUUID,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return manager, nil
}

func (ws *workers) txnLogWatcher() watcher.BaseWatcher {
	w, err := ws.Worker(txnLogWorker, nil)
	if err != nil {
		return watcher.NewDead(errors.Trace(err))
	}
	return w.(watcher.BaseWatcher)
}

func (ws *workers) presenceWatcher() *presence.Watcher {
	w, err := ws.Worker(presenceWorker, nil)
	if err != nil {
		return presence.NewDeadWatcher(errors.Trace(err))
	}
	return w.(*presence.Watcher)
}

func (ws *workers) pingBatcherWorker() *presence.PingBatcher {
	w, err := ws.Worker(pingBatcherWorker, nil)
	if err != nil {
		return presence.NewDeadPingBatcher(errors.Trace(err))
	}
	return w.(*presence.PingBatcher)
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
	if err := ws.StartWorker(allManagerWorker, func() (worker.Worker, error) {
		// The allWatcher uses system state to load all the entities to watch.
		// The system state is not ref counted like other states that are fetched
		// from the state pool, so create a copy here and use that to guard against
		// the possibility that the system state may be closed elsewhere.
		// The state copy is closed then the store manager is released.
		session := ws.state.session.Copy()
		stCopy, err := newState(
			ws.state.modelTag,
			ws.state.controllerModelTag,
			session,
			ws.state.newPolicy,
			ws.state.stateClock,
			ws.state.runTransactionObserver,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return newStoreManager(newAllWatcherStateBacking(stCopy, params)), nil
	}); err != nil {
		newDeadStoreManager(errors.Trace(err))
	}
	// Yes, this is recursive but it will exit early above as the
	// worker will now be created.
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
	if err := ws.StartWorker(allModelManagerWorker, func() (worker.Worker, error) {
		// The allWatcher uses system state to load all the entities to watch.
		// The system state is not ref counted like other states that are fetched
		// from the state pool, so create a copy here and use that to guard against
		// the possibility that the system state may be closed elsewhere.
		// The state copy is closed then the store manager is released.
		session := ws.state.session.Copy()
		stCopy, err := newState(
			ws.state.modelTag,
			ws.state.controllerModelTag,
			session,
			ws.state.newPolicy,
			ws.state.stateClock,
			ws.state.runTransactionObserver,
		)
		if err != nil {
			return nil, errors.Trace(err)
		}

		return newStoreManager(NewAllModelWatcherStateBacking(stCopy, pool)), nil
	}); err != nil {
		return newDeadStoreManager(errors.Trace(err))
	}
	// Yes, this is recursive but it will exit early above as the
	// worker will now be created.
	return ws.allModelManager(pool)
}

// lazyLeaseManager wraps one of workers.singularManager or
// workers.leadershipManager, and calls it in the method calls.
// This enables the manager to use restarted lease managers.
type lazyLeaseManager struct {
	leaseManager func() *lease.Manager
}

// Claim is part of the lease.Claimer interface.
func (l lazyLeaseManager) Claim(leaseName, holderName string, duration time.Duration) error {
	return l.leaseManager().Claim(leaseName, holderName, duration)
}

// WaitUntilExpired is part of the lease.Claimer interface.
func (l lazyLeaseManager) WaitUntilExpired(leaseName string, cancel <-chan struct{}) error {
	return l.leaseManager().WaitUntilExpired(leaseName, cancel)
}

// Token is part of the lease.Checker interface.
func (l lazyLeaseManager) Token(leaseName, holderName string) corelease.Token {
	return l.leaseManager().Token(leaseName, holderName)
}
