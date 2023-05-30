// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventsource

import (
	"github.com/juju/juju/core/changestream"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/database"
)

// BaseWatcher encapsulates members common to all EventQueue-based watchers.
// It has no functionality by itself, and is intended to be embedded in
// other more specific watchers.
type BaseWatcher struct {
	tomb tomb.Tomb

	eventSource changestream.EventSource
	db          database.TxnRunner
	logger      Logger
}

// NewBaseWatcher returns a BaseWatcher constructed from the arguments.
func NewBaseWatcher(eq changestream.EventSource, db database.TxnRunner, l Logger) *BaseWatcher {
	return &BaseWatcher{
		eventSource: eq,
		db:          db,
		logger:      l,
	}
}
