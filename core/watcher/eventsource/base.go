// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/juju/core/changestream"
	"gopkg.in/tomb.v2"
)

// BaseWatcher encapsulates members common to all EventQueue-based watchers.
// It has no functionality by itself, and is intended to be embedded in
// other more specific watchers.
type BaseWatcher struct {
	tomb tomb.Tomb

	watchableDB changestream.WatchableDB
	logger      Logger
}

// NewBaseWatcher returns a BaseWatcher constructed from the arguments.
func NewBaseWatcher(watchableDB changestream.WatchableDB, l Logger) *BaseWatcher {
	return &BaseWatcher{
		watchableDB: watchableDB,
		logger:      l,
	}
}
