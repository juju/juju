// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/unit"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/internal/errors"
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
	// NewNamespaceMapperWatcher returns a new watcher that receives changes
	// from the input base watcher's db/queue. Change-log events will be emitted
	// only if the filter accepts them, and dispatching the notifications via
	// the Changes channel, once the mapper has processed them. Filtering of
	// values is done first by the filter, and then by the mapper. Based on the
	// mapper's logic a subset of them (or none) may be emitted. A filter option
	// is required, though additional filter options can be provided.
	NewNamespaceMapperWatcher(
		initialQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyMapperWatcher returns a new watcher that receives changes from
	// the input base watcher's db/queue. A single filter option is required,
	// though additional filter options can be provided. Filtering of values is
	// done first by the filter, and then subsequently by the mapper. Based on
	// the mapper's logic a subset of them (or none) may be emitted.
	NewNotifyMapperWatcher(
		mapper eventsource.Mapper,
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// WatcherState describes the methods that the service needs for its watchers.
type WatcherState interface {
	// NamespaceForWatchOpenedPort returns the name of the table that should be
	// watched
	NamespaceForWatchOpenedPort() string

	// InitialWatchMachineOpenedPortsStatement returns the name of the table
	// that should be watched and the query to load the initial event for the
	// WatchMachineOpenedPorts watcher.
	InitialWatchMachineOpenedPortsStatement() (string, string)

	// GetMachineNamesForUnits returns map from endpoint uuids to the uuids of
	// the machines which host that endpoint for each provided endpoint uuid.
	GetMachineNamesForUnits(context.Context, []unit.UUID) ([]coremachine.Name, error)

	// FilterUnitUUIDsForApplication returns the subset of provided endpoint
	// uuids that are associated with the provided application.
	FilterUnitUUIDsForApplication(context.Context, []unit.UUID, coreapplication.ID) (set.Strings, error)
}

// WatchMachineOpenedPorts returns a strings watcher for opened ports. This
// watcher emits events for changes to the opened ports table. Each emitted
// event contains the machine name which is associated with the changed port
// range.
func (s *WatchableService) WatchMachineOpenedPorts(ctx context.Context) (_ watcher.StringsWatcher, err error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	table, statement := s.st.InitialWatchMachineOpenedPortsStatement()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		eventsource.InitialNamespaceChanges(statement),
		s.endpointToMachineMapper,
		eventsource.NamespaceFilter(table, changestream.All),
	)
}

// WatchOpenedPortsForApplication returns a notify watcher for opened ports. This
// watcher emits events for changes to the opened ports table that are associated
// with the given application
func (s *WatchableService) WatchOpenedPortsForApplication(ctx context.Context, applicationUUID coreapplication.ID) (_ watcher.NotifyWatcher, err error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer func() {
		span.RecordError(err)
		span.End()
	}()

	return s.watcherFactory.NewNotifyMapperWatcher(
		s.filterForApplication(applicationUUID),
		eventsource.NamespaceFilter(s.st.NamespaceForWatchOpenedPort(), changestream.All),
	)
}

// endpointToMachineMapper maps endpoint uuids to the machine names that host
// the endpoint.
func (s *WatchableService) endpointToMachineMapper(
	ctx context.Context, events []changestream.ChangeEvent,
) ([]changestream.ChangeEvent, error) {
	if len(events) == 0 {
		return nil, nil
	}

	var indexes []indexed
	unique := make(map[unit.UUID]struct{})
	for _, event := range events {
		id, err := unit.ParseID(event.Changed())
		if err != nil {
			return nil, err
		}

		indexes = append(indexes, indexed{event: event, id: id})
		unique[id] = struct{}{}
	}

	// This is suboptimal, we need the unit.UUID in the result so we can map
	// it back to the id.
	reference := make(map[unit.UUID]coremachine.Name)
	for id := range unique {
		machineNames, err := s.st.GetMachineNamesForUnits(ctx, []unit.UUID{id})
		if err != nil {
			return nil, err
		} else if len(machineNames) != 1 {
			return nil, errors.Errorf("expected exactly one machine for unit %q, got %d", id, len(machineNames))
		}

		reference[id] = machineNames[0]
	}

	result := make([]changestream.ChangeEvent, len(indexes))
	for i, event := range indexes {
		name, ok := reference[event.id]
		if !ok {
			return nil, errors.Errorf("no machine found for unit %q", event.id)
		}

		result[i] = maskedChangeEvent{
			ChangeEvent: event.event,
			namespace:   "machine",
			machineName: name.String(),
		}
	}

	return result, nil
}

type indexed struct {
	event changestream.ChangeEvent
	id    unit.UUID
}

// filterForApplication returns an eventsource.Mapper that filters events
// emitted by port range changes to only include events for port range changes
// corresponding to the given application
func (s *WatchableService) filterForApplication(applicationUUID coreapplication.ID) eventsource.Mapper {
	return func(
		ctx context.Context, events []changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		unitUUIDs, err := transform.SliceOrErr(events, func(e changestream.ChangeEvent) (unit.UUID, error) {
			return unit.ParseID(e.Changed())
		})
		if err != nil {
			return nil, err
		}

		unitUUIDsForApplication, err := s.st.FilterUnitUUIDsForApplication(ctx, unitUUIDs, applicationUUID)
		if err != nil {
			return nil, err
		}
		results := make([]changestream.ChangeEvent, 0, len(events))
		for _, event := range events {
			if unitUUIDsForApplication.Contains(event.Changed()) {
				results = append(results, event)
			}
		}
		return results, nil
	}
}

// maskedChangeEvent implements changestream.ChangeEvent and allows the
// substituting of events in an implementation of eventsource.Mapper.
type maskedChangeEvent struct {
	changestream.ChangeEvent
	namespace   string
	machineName string
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (e maskedChangeEvent) Namespace() (_ string) {
	return e.namespace
}

// Changed returns the changed value of event. This logically can be
// the primary key of the row that was changed or the field of the change
// that was changed.
func (e maskedChangeEvent) Changed() (_ string) {
	return e.machineName
}
