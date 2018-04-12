// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/watcher"
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
	Charm(application string) (_ *charm.URL, force bool, sha256 string, vers int, _ error)
}

type applicationWatcher interface {
	// Watch returns a watcher that fires when the application changes.
	Watch(application string) (watcher.NotifyWatcher, error)
}
