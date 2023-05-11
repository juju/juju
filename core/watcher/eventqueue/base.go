// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package eventqueue

import (
	"gopkg.in/tomb.v2"
)

// BaseWatcher encapsulates members common to all EventQueue-based watchers.
// It has no functionality by itself, and is intended to be embedded in
// other more specific watchers.
type BaseWatcher struct {
	tomb tomb.Tomb

	eventQueue EventQueue
	db         TrackedDB
	logger     Logger
}

// NewBaseWatcher returns a BaseWatcher constructed from the arguments.
func NewBaseWatcher(eq EventQueue, db TrackedDB, l Logger) *BaseWatcher {
	return &BaseWatcher{
		eventQueue: eq,
		db:         db,
		logger:     l,
	}
}
