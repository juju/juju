// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
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
func NewWatchableService(st State, watcherFactory WatcherFactory, logger logger.Logger) *WatchableService {
	return &WatchableService{
		Service:        &Service{st: st, logger: logger},
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

	// InitialWatchMachineOpenedPortsStatement returns the query to load the initial
	// event for the WatchMachineOpenedPorts watcher
	InitialWatchMachineOpenedPortsStatement() string

	// InitialWatchApplicationOpenedPortsStatement returns the query to load the
	// initial event for the WatchApplicationOpenedPorts watcher
	InitialWatchApplicationOpenedPortsStatement() string

	// GetMachineNamesForUnitEndpoints returns map from endpoint uuids to the uuids of
	// the machines which host that endpoint for each provided endpoint uuid.
	GetMachineNamesForUnitEndpoints(ctx context.Context, endpointUUIDs []string) ([]coremachine.Name, error)

	// GetApplicationNamesForUnitEndpoints returns a slice of application names that
	// host the provided endpoints.
	GetApplicationNamesForUnitEndpoints(ctx context.Context, eps []string) ([]string, error)
}

// WatchOpenedPorts returns a strings watcher for opened ports. This watcher
// emits events for changes to the opened ports table. Each emitted event
// contains the machine name which is associated with the changed port range.
func (s *WatchableService) WatchMachineOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceMapperWatcher(
		s.st.WatchOpenedPortsTable(),
		changestream.Create|changestream.Delete,
		eventsource.InitialNamespaceChanges(s.st.InitialWatchMachineOpenedPortsStatement()),
		s.endpointToMachineMapper,
	)
}

// WatchApplicationOpenedPorts returns a strings watcher for opened ports. This
// watcher emits for changes to the opened ports table. Each emitted event contains
// the app name which is associated with the changed port range.
func (s *WatchableService) WatchApplicationOpenedPorts(ctx context.Context) (watcher.StringsWatcher, error) {
	return s.watcherFactory.NewNamespaceMapperWatcher(
		s.st.WatchOpenedPortsTable(),
		changestream.Create|changestream.Delete,
		eventsource.InitialNamespaceChanges(s.st.InitialWatchApplicationOpenedPortsStatement()),
		s.endpointToApplicationMapper,
	)
}

// endpointToMachineMapper is an eventsource.Mapper that maps a slice of
// changestream.ChangeEvent containing endpoint UUIDs to a slice of
// changestream.ChanegEvent containing the names of the machines the corresponding
// endpoint is located on.
func (s *WatchableService) endpointToMachineMapper(
	ctx context.Context, db database.TxnRunner, events []changestream.ChangeEvent,
) ([]changestream.ChangeEvent, error) {

	endpointUUIDs := transform.Slice(events, func(e changestream.ChangeEvent) string {
		return e.Changed()
	})

	machineNames, err := s.st.GetMachineNamesForUnitEndpoints(ctx, endpointUUIDs)
	if err != nil {
		return nil, err
	}

	newEvents := make([]changestream.ChangeEvent, 0, len(machineNames))
	for _, machineName := range machineNames {
		newEvents = append(newEvents, changeEventShim{
			changeType: changestream.Update,
			namespace:  "machine",
			changed:    machineName.String(),
		})
	}

	return newEvents, nil
}

func (s *WatchableService) endpointToApplicationMapper(
	ctx context.Context, db database.TxnRunner, events []changestream.ChangeEvent,
) ([]changestream.ChangeEvent, error) {

	endpointUUIDs := transform.Slice(events, func(e changestream.ChangeEvent) string {
		return e.Changed()
	})

	applicationNames, err := s.st.GetApplicationNamesForUnitEndpoints(ctx, endpointUUIDs)
	if err != nil {
		return nil, err
	}

	newEvents := make([]changestream.ChangeEvent, 0, len(applicationNames))
	for _, applicationName := range applicationNames {
		newEvents = append(newEvents, changeEventShim{
			changeType: changestream.Update,
			namespace:  "application",
			changed:    applicationName,
		})
	}

	return newEvents, nil
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
