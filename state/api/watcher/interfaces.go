// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

// NotifyWatcher will send events when something changes.
// It does not send content for those changes.
type NotifyWatcher interface {
	Changes() <-chan struct{}
	Stop() error
	Err() error
}

// StringsWatcher will send events when something changes.
// The content for the changes is a list of strings.
type StringsWatcher interface {
	Changes() <-chan []string
	Stop() error
	Err() error
}
