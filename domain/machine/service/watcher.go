// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
)

type WatchableService struct {
	// Machine Service
	Service
	watcherFactory WatcherFactory
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewUUIDsWatcher returns a watcher that emits the UUIDs for
	// changes to the input table name that match the input mask.
	NewUUIDsWatcher(
		tableName string, changeMask changestream.ChangeType,
	) (watcher.StringsWatcher, error)
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service: Service{
			st: st,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchMachines returns a StringsWatcher that is subscribed to the changes
// in the machines table in the model.
func (s *WatchableService) WatchMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewUUIDsWatcher(
		"machine",
		changestream.All,
	)
}
