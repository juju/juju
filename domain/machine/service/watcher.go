// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

type WatchableService struct {
	// Machine Service
	Service
	watcherFactory WatcherFactory
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(
		namespace string, changeMask changestream.ChangeType,
		initialStateQuery eventsource.NamespaceQuery,
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
	table, stmt := s.st.InitialWatchStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		table,
		changestream.All,
		eventsource.InitialNamespaceChanges(stmt),
	)
}
