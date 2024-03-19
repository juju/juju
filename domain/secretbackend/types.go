// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackend

import (
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(string, changestream.ChangeType, string) (watcher.StringsWatcher, error)
}
