// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"path"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/pubsub"
	"gopkg.in/juju/worker.v1"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/state/watcher"
	jworker "github.com/juju/juju/worker"
	"github.com/juju/juju/worker/lease"
)

const (
	txnLogWorker          = "txnlog"
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
	//	model *Model
	*worker.Runner

	hub *pubsub.SimpleHub
}

const pingFlushInterval = time.Second

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
	ws.StartWorker(txnLogWorker, func() (worker.Worker, error) {
		return watcher.NewHubWatcher(watcher.HubWatcherConfig{
			Hub:       hub,
			Clock:     st.clock(),
			ModelUUID: st.modelUUID(),
			Logger:    loggo.GetLogger("juju.state.watcher"),
		})
	})
	ws.StartWorker(leadershipWorker, func() (worker.Worker, error) {
		manager, err := st.newLeaseManager(st.getLeadershipLeaseStore, lease.LeadershipSecretary{}, st.ModelUUID())
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	})
	ws.StartWorker(singularWorker, func() (worker.Worker, error) {
		manager, err := st.newLeaseManager(st.getSingularLeaseStore,
			lease.SingularSecretary{
				ControllerUUID: st.ControllerUUID(),
			},
			st.ControllerUUID(),
		)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return manager, nil
	})
	return ws, nil
}

func (st *State) newLeaseManager(
	getStore func() (corelease.Store, error),
	secretary lease.Secretary,
	entityUUID string,
) (worker.Worker, error) {
	store, err := getStore()
	if err != nil {
		return nil, errors.Trace(err)
	}
	series, err := series.HostSeries()
	if err != nil {
		return nil, errors.Trace(err)
	}
	logDir, err := paths.LogDir(series)
	if err != nil {
		return nil, errors.Trace(err)
	}
	manager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: func(_ string) (lease.Secretary, error) {
			return secretary, nil
		},
		Store:      store,
		Clock:      st.clock(),
		Logger:     loggo.GetLogger("juju.worker.lease.mongo"),
		MaxSleep:   time.Minute,
		EntityUUID: entityUUID,
		LogDir:     path.Join(logDir, "juju"),
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

// lazyLeaseClaimer wraps one of workers.singularManager.Claimer or
// workers.leadershipManager.Claimer, and calls it in the method
// calls. This enables the manager to use restarted lease managers.
type lazyLeaseClaimer struct {
	leaseClaimer func() (corelease.Claimer, error)
}

// Claim is part of the lease.Claimer interface.
func (l lazyLeaseClaimer) Claim(leaseName, holderName string, duration time.Duration) error {
	claimer, err := l.leaseClaimer()
	if err != nil {
		return errors.Trace(err)
	}
	return claimer.Claim(leaseName, holderName, duration)
}

// WaitUntilExpired is part of the lease.Claimer interface.
func (l lazyLeaseClaimer) WaitUntilExpired(leaseName string, cancel <-chan struct{}) error {
	claimer, err := l.leaseClaimer()
	if err != nil {
		return errors.Trace(err)
	}
	return claimer.WaitUntilExpired(leaseName, cancel)
}

// lazyLeaseChecker wraps one of workers.singularManager.Checker or
// workers.leadershipManager.Checker, and calls it in the method
// calls. This enables the manager to use restarted lease managers.
type lazyLeaseChecker struct {
	leaseChecker func() (corelease.Checker, error)
}

// Token is part of the lease.Checker interface.
func (l lazyLeaseChecker) Token(leaseName, holderName string) corelease.Token {
	checker, err := l.leaseChecker()
	if err != nil {
		return errorToken{err: errors.Trace(err)}
	}
	return checker.Token(leaseName, holderName)
}

// errorToken is a token whose Check method always returns the given
// error.
type errorToken struct {
	err error
}

// Check is part of the lease.Token interface.
func (t errorToken) Check(attempt int, key interface{}) error {
	return t.err
}
