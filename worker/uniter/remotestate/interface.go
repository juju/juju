// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

type Watcher interface {
	// RemoteStateChanged returns a channel which is signalled
	// whenever the remote state is changed.
	RemoteStateChanged() <-chan struct{}

	// Snapshot returns the current snapshot of the remote state.
	Snapshot() Snapshot

	// TODO(axw) Kill instead of Stop
	Stop() error
	Wait() error
}
