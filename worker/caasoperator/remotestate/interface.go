// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/worker/v2"

	caasoperatorapi "github.com/juju/juju/api/caasoperator"
	"github.com/juju/juju/core/watcher"
)

type Watcher interface {
	// RemoteStateChanged returns a channel which is signalled
	// whenever the remote state is changed.
	RemoteStateChanged() <-chan struct{}

	// Snapshot returns the current snapshot of the remote state.
	Snapshot() Snapshot

	worker.Worker
}

type charmGetter interface {
	Charm(application string) (*caasoperatorapi.CharmInfo, error)
}

type applicationWatcher interface {
	// Watch returns a watcher that fires when the application changes.
	Watch(application string) (watcher.NotifyWatcher, error)
}
