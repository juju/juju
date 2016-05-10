// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workers

import (
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/state/presence"
	"github.com/juju/juju/state/watcher"
	"github.com/juju/juju/worker"
)

// Workers doesn't really need to exist -- it could basically just exist
// in the state package -- but that'd entail duplication of the
// TxnLogWatcher, PresenceWatcher, and LeaseManager interfaces in both
// packages to avoid import cycles, and, yuck.
//
// See the DumbWorkers and RestartWorkers types for implementations.
type Workers interface {
	TxnLogWatcher() TxnLogWatcher
	PresenceWatcher() PresenceWatcher
	LeadershipManager() LeaseManager
	SingularManager() LeaseManager

	// Kill causes the lease manager workers to start shutting down,
	// and not to be restarted. See the client (HackLeadership) for
	// an explanation.
	Kill()

	// Stop stops all the workers and returns any error encountered.
	Stop() error
}

// Factory supplies implementations of various workers used in state,
// and is generally a critical dependency of a Workers implementation
// such as DumbWorkers or RestartWorkers.
//
// It'll generally just be a thin wrapper over a *State -- this package
// exists only to paper over worker-lifetime issues that are hard to
// address in the state package, not really to pave the way to alternate
// backends or anything.
type Factory interface {
	NewTxnLogWorker() (TxnLogWorker, error)
	NewPresenceWorker() (PresenceWorker, error)
	NewLeadershipWorker() (LeaseWorker, error)
	NewSingularWorker() (LeaseWorker, error)
}

// TxnLogWatcher exposes the methods of watcher.Watcher that are needed
// by the state package.
type TxnLogWatcher interface {

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

// TxnLogWorker includes the watcher.Watcher's worker.Worker methods,
// so that a Workers implementation can manage its lifetime.
type TxnLogWorker interface {
	worker.Worker
	TxnLogWatcher
}

// PresenceWatcher exposes the methods of presence.Watcher that are
// needed by the state package.
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

// PresenceWorker includes the presence.Watcher's worker.Worker methods,
// so that a Workers implementation can manage its lifetime.
type PresenceWorker interface {
	worker.Worker
	PresenceWatcher
}

// LeaseManager exposes the methods of lease.Manager that are needed by
// the state package.
type LeaseManager interface {
	lease.Claimer
	lease.Checker
}

// LeaseWorker includes the lease.Manager's worker.Worker methods,
// so that a Workers implementation can manage its lifetime.
type LeaseWorker interface {
	worker.Worker
	LeaseManager
}
