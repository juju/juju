// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// WatchableService provides the API for managing the opened ports for units, as
// well as the ability to watch for changes to opened ports.
type WatchableService struct {
	*Service
	watcherFactory WatcherFactory
}

// NewWatchableService returns a new Service providing an API to manage the opened
// ports for units.
func NewWatchableService(st State, watcherFactory WatcherFactory) *WatchableService {
	return &WatchableService{
		Service:        &Service{st: st},
		watcherFactory: watcherFactory,
	}
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceMapperWatcher returns a new namespace watcher
	// for events based on the input change mask and mapper.
	NewNamespaceMapperWatcher(
		namespace string,
		changeMask changestream.ChangeType,
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
	) (watcher.StringsWatcher, error)

	// NewNamespaceNotifyMapperWatcher returns a new namespace notify watcher
	// for events based on the input change mask and mapper.
	NewNamespaceNotifyMapperWatcher(
		namespace string, changeMask changestream.ChangeType, mapper eventsource.Mapper,
	) (watcher.NotifyWatcher, error)
}

// WatcherState describes the methods that the service needs for its watchers.
type WatcherState interface {
	// WatchOpenedPortsTable returns the name of the table that should be watched
	WatchOpenedPortsTable() string

	// InitialWatchOpenedPortsStatement returns the query to load the initial event
	// for the WatchOpenedPorts watcher
	InitialWatchOpenedPortsStatement() string

	// GetMachinesForEndpoints returns map from endpoint uuids to the uuids of
	// the machines which host that endpoint for each provided endpoint uuid.
	GetMachinesForEndpoints(ctx context.Context, endpointUUIDs []string) ([]string, error)

	// FilterEndpointsForApplication returns the subset of provided endpoint uuids
	// that are associated with the provided application.
	FilterEndpointsForApplication(ctx context.Context, app coreapplication.ID, eps []string) (set.Strings, error)
}

// WatchOpenedPorts returns a strings watcher for opened ports. This watcher
// emits events for changes to the opened ports table. Each emitted event
// contains the machine UUID which is associated with the port range.
func (s *WatchableService) WatchOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceMapperWatcher(
		s.st.WatchOpenedPortsTable(),
		changestream.Create|changestream.Delete,
		eventsource.InitialNamespaceChanges(s.st.InitialWatchOpenedPortsStatement()),
		s.endpointToMachineMapper,
	)
}

// WatchOpenedPortsForApplication returns a notify watcher for opened ports. This
// watcher emits events for changes to the opened ports table that are associated
// with the given application
func (s *WatchableService) WatchOpenedPortsForApplication(ctx context.Context, applicationUUID coreapplication.ID) (watcher.NotifyWatcher, error) {
	return s.watcherFactory.NewNamespaceNotifyMapperWatcher(
		s.st.WatchOpenedPortsTable(),
		changestream.Create|changestream.Delete,
		s.filterForApplication(applicationUUID),
	)
}

// endpointToMachineMapper is an eventsource.Mapper that maps a slice of
// changestream.ChangeEvent containing endpoint UUIDs to a slice of
// changestream.ChanegEvent containing the UUIDs of the machines the corresponding
// endpoint is located on.
func (s *WatchableService) endpointToMachineMapper(
	ctx context.Context, db database.TxnRunner, events []changestream.ChangeEvent,
) ([]changestream.ChangeEvent, error) {

	endpointUUIDs := transform.Slice(events, func(e changestream.ChangeEvent) string {
		return e.Changed()
	})

	machineUUIDs, err := s.st.GetMachinesForEndpoints(ctx, endpointUUIDs)
	if err != nil {
		return nil, err
	}

	newEvents := make([]changestream.ChangeEvent, 0, len(machineUUIDs))
	for _, machineUUID := range machineUUIDs {
		newEvents = append(newEvents, changeEventShim{
			changeType: changestream.Update,
			namespace:  "machine",
			changed:    machineUUID,
		})
	}

	return newEvents, nil
}

// filterForApplication returns an eventsource.Mapper that filters events emitted
// by port range changes to only include events for port range changes corresponding
// to the given application
func (s *WatchableService) filterForApplication(applicationUUID coreapplication.ID) eventsource.Mapper {
	return func(
		ctx context.Context, db database.TxnRunner, events []changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		endpointUUIDs := transform.Slice(events, func(e changestream.ChangeEvent) string {
			return e.Changed()
		})
		endpointUUIDsForApplication, err := s.st.FilterEndpointsForApplication(ctx, applicationUUID, endpointUUIDs)
		if err != nil {
			return nil, err
		}
		results := make([]changestream.ChangeEvent, 0, len(events))
		for _, event := range events {
			if endpointUUIDsForApplication.Contains(event.Changed()) {
				results = append(results, event)
			}
		}
		return results, nil
	}
}

// changeEventShim implements changestream.ChangeEvent and allows the
// substituting of events in an implementation of eventsource.Mapper.
type changeEventShim struct {
	changeType changestream.ChangeType
	namespace  string
	changed    string
}

// Type returns the type of change (create, update, delete).
func (e changeEventShim) Type() changestream.ChangeType {
	return e.changeType
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (e changeEventShim) Namespace() string {
	return e.namespace
}

// Changed returns the changed value of event. This logically can be
// the primary key of the row that was changed or the field of the change
// that was changed.
func (e changeEventShim) Changed() string {
	return e.changed
}
