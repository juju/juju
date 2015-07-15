// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state/multiwatcher"
)

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

// EntityWatcher will send events when something changes.
// The content for the changes is a list of tag strings.
type EntityWatcher interface {
	Changes() <-chan []string
	Stop() error
	Err() error
}

// RelationUnitsWatcher will send events when something changes.
// The content for the changes is a params.RelationUnitsChange struct.
type RelationUnitsWatcher interface {
	Changes() <-chan multiwatcher.RelationUnitsChange
	Stop() error
	Err() error
}

// MachineStorageIdsWatcher will send events when the lifecycle states
// of machine/storage entities change. The content for the changes is a
// list of params.MachineStorageId.
type MachineStorageIdsWatcher interface {
	Changes() <-chan []params.MachineStorageId
	Stop() error
	Err() error
}
