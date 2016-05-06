// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/utils/clock"
	"gopkg.in/mgo.v2"

	corelease "github.com/juju/juju/core/lease"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker/lease"
)

// TxnWatcher merely wraps a state/watcher.Watcher for future
// flexibility, specifically excluding the ability to stop it
// (which is not needed/wanted by this type's clients)
type TxnWatcher interface {

	// pseudo-workery bits, ideally to be replaced one day?
	Err() error
	Dead() <-chan struct{}

	// horrible hack for goosing it into activity
	StartSync()

	// single-document watching
	Watch(coll string, id interface{}, revno int64, ch chan<- watcher.Change)
	Unwatch(coll string, id interface{}, ch chan<- watcher.Change)

	// collection-watching
	WatchCollection(coll string, ch chan<- watcher.Change)
	WatchCollectionWithFilter(coll string, ch chan<- watcher.Change, filter func(interface{}) bool)
	UnwatchCollection(coll string, ch chan<- watcher.Change)
}

// PresenceWatcher merely wraps a state/presence.Watcher for future
// flexibility, specifically excluding the ability to stop it (which
// is not needed/wanted by this type's clients).
type PresenceWatcher interface {

	// pseudo-workery bits, ideally to be replaced one day?
	Err() error
	Dead() <-chan struct{}

	// horrible hack for goosing it into activity. not clear why
	// this is used in place of StartSync, but it is.
	Sync()

	// presence-reading and -watching
	Alive(key string) (bool, error)
	Watch(key string, ch chan<- presence.Change)
	Unwatch(key string, ch chan<- presence.Change)
}

// LeaseManager combines corelease.Claimer and corelease.Checker for the
// convenience of leadership/singular-controller logic.
type LeaseManager interface {
	corelease.Claimer
	corelease.Checker
}

// Workers exposes implementations of various capabilities that have the
// unhelpful property of being able to fail independently of state
// itself. It's meant to accommodate future implementations that
// automatically restart workers (or supply always-failing worker stubs
// to clients when no worker can be found or created).
type Workers interface {
	TxnWatcher() TxnWatcher
	PresenceWatcher() PresenceWatcher
	LeadershipManager() LeaseManager
	SingularManager() LeaseManager

	// Stop stops all the workers and returns any error encountered.
	Stop() error

	// Kill causes the lease manager workers to start shutting down,
	// and not to be restarted. See the client (HackLeadership) for
	// an explanation.
	Kill()
}

// dumbWorkersConfig holds information necessary to construct all the
// standard state workers. It's not set up to restart workers in any
// way; it just knows enough to launch them once and leave them alone,
// exactly as we did on State before this change.
type dumbWorkersConfig struct {
	modelTag           names.ModelTag
	clock              clock.Clock
	leadershipClient   corelease.Client
	singularClient     corelease.Client
	presenceCollection *mgo.Collection
	txnLogCollection   *mgo.Collection
}

// newDumbWorkers returns a collection of standard state workers that
// are not robust in any way: if they fail, they fail, and nothing will
// restart them. It exists as a refectoring step, and should be replaced
// with a smarter implementation that recreates, retries, and supplies
// cleanly-  and clearly-failing versions when unable.
func newDumbWorkers(config dumbWorkersConfig) (_ *dumbWorkers, err error) {

	result := &dumbWorkers{}
	defer func() {
		if err != nil {
			if stopErr := result.Stop(); stopErr != nil {
				logger.Errorf("while aborting dumbWorkers creation: %v", err)
			}
		}
	}()

	logger.Infof("starting leadership lease manager")
	leadershipManager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: leadershipSecretary{},
		Client:    config.leadershipClient,
		Clock:     config.clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create leadership lease manager")
	}
	result.leadershipManager = leadershipManager

	logger.Infof("starting singular lease manager")
	singularManager, err := lease.NewManager(lease.ManagerConfig{
		Secretary: singularSecretary{config.modelTag.Id()},
		Client:    config.singularClient,
		Clock:     config.clock,
		MaxSleep:  time.Minute,
	})
	if err != nil {
		return nil, errors.Annotatef(err, "cannot create singular lease manager")
	}
	result.singularManager = singularManager

	logger.Infof("starting transaction log watcher")
	result.txnLogWatcher = watcher.New(config.txnLogCollection)

	logger.Infof("starting presence watcher")
	result.presenceWatcher = presence.NewWatcher(
		config.presenceCollection, config.modelTag,
	)
	return result, nil
}

// dumbWorkers holds references to standard state workers.
type dumbWorkers struct {
	txnLogWatcher     *watcher.Watcher
	presenceWatcher   *presence.Watcher
	leadershipManager *lease.Manager
	singularManager   *lease.Manager
}

// Stop is part of the Workers interface.
func (dw *dumbWorkers) Stop() error {
	var errs []error
	handle := func(name string, err error) {
		if err != nil {
			errs = append(errs, errors.Annotatef(err, "error stopping %s", name))
		}
	}

	if dw.txnLogWatcher != nil {
		handle("transaction watcher", dw.txnLogWatcher.Stop())
	}
	if dw.presenceWatcher != nil {
		handle("presence watcher", dw.presenceWatcher.Stop())
	}
	if dw.leadershipManager != nil {
		dw.leadershipManager.Kill()
		handle("leadership manager", dw.leadershipManager.Wait())
	}
	if dw.singularManager != nil {
		dw.singularManager.Kill()
		handle("singular manager", dw.singularManager.Wait())
	}

	if len(errs) > 0 {
		for _, err := range errs[1:] {
			logger.Errorf("while stopping state workers: %v", err)
		}
		return errs[0]
	}
	logger.Debugf("stopped state workers without error")
	return nil
}

// Kill is part of the Workers interface.
func (dw *dumbWorkers) Kill() {
	dw.leadershipManager.Kill()
	dw.singularManager.Kill()
}

// TxnWatcher is part of the Workers interface.
func (dw *dumbWorkers) TxnWatcher() TxnWatcher {
	return dw.txnLogWatcher
}

// PresenceWatcher is part of the Workers interface.
func (dw *dumbWorkers) PresenceWatcher() PresenceWatcher {
	return dw.presenceWatcher
}

// LeadershipManager is part of the Workers interface.
func (dw *dumbWorkers) LeadershipManager() LeaseManager {
	return dw.leadershipManager
}

// SingularManager is part of the Workers interface.
func (dw *dumbWorkers) SingularManager() LeaseManager {
	return dw.singularManager
}
