// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

// Watcher is implemented by all watchers; the actual
// changes channel is returned by a watcher-specific
// Changes method.
type Watcher interface {
	// Kill asks the watcher to stop without waiting for it do so.
	Kill()
	// Wait waits for the watcher to die and returns any
	// error encountered when it was running.
	Wait() error
	// Stop kills the watcher, then waits for it to die.
	Stop() error
	// Err returns any error encountered while the watcher
	// has been running.
	Err() error
}

// NotifyWatcher generates signals when something changes, but it does not
// return any content for those changes
type NotifyWatcher interface {
	Watcher
	Changes() <-chan struct{}
}

// StringsWatcher generates signals when something changes, returning
// the changes as a list of strings.
type StringsWatcher interface {
	Watcher
	Changes() <-chan []string
}
