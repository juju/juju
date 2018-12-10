// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"gopkg.in/juju/worker.v1"
)

// The watchers used in the cache package are closer to state watchers
// than core watchers. The core watchers never close their changes channel,
// which leads to issues in the apiserver facade methods dealing with
// watchers. So the watchers in this package do close their changes channels.

// Watcher is the common methods
type Watcher interface {
	worker.Worker
	// Stop is currently needed by the apiserver until the resources
	// work on workers instead of things that can be stopped.
	Stop() error
}

// NotifyWatcher will only say something changed.
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}
