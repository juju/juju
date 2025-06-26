// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"strings"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// WatchableService is a service that can be watched for updates to machine
// values.
type WatchableService struct {
	ProviderService
	watcherFactory WatcherFactory
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
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
		filterOption eventsource.FilterOption, filterOptions ...eventsource.FilterOption,
	) (watcher.StringsWatcher, error)

	// NewNotifyWatcher returns a new watcher that filters changes from the
	// input base watcher's db/queue. A single filter option is required, though
	// additional filter options can be provided.
	NewNotifyWatcher(
		filter eventsource.FilterOption,
		filterOpts ...eventsource.FilterOption,
	) (watcher.NotifyWatcher, error)
}

// NewWatchableService returns a new service reference wrapping the input state.
func NewWatchableService(
	st State,
	watcherFactory WatcherFactory,
	providerGetter providertracker.ProviderGetter[Provider],
	statusHistory StatusHistory,
	clock clock.Clock,
	logger logger.Logger,
) *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{
				st:            st,
				statusHistory: statusHistory,
				clock:         clock,
				logger:        logger,
			},
			providerGetter: providerGetter,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchMachineLife returns a watcher that observes the changes to life of one
// machine.
func (s *WatchableService) WatchMachineLife(ctx context.Context, machineName machine.Name) (watcher.NotifyWatcher, error) {
	table := s.st.NamespaceForMachineLife()
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			table,
			changestream.All,
			eventsource.EqualsPredicate(machineName.String()),
		),
	)
}

// WatchModelMachines watches for additions or updates to non-container
// machines. It is used by workers that need to factor life value changes,
// and so does not factor machine removals, which are considered to be
// after their transition to the dead state.
// It emits machine names rather than UUIDs.
func (s *WatchableService) WatchModelMachines(ctx context.Context) (watcher.StringsWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	table, stmt := s.st.InitialWatchModelMachinesStatement()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		s.uuidToNameMapper(noContainersFilter),
		eventsource.NamespaceFilter(table, changestream.Changed),
	)
}

// WatchMachineCloudInstances returns a NotifyWatcher that is subscribed to
// the changes in the machine_cloud_instance table in the model, for the given
// machine UUID.
func (s *WatchableService) WatchMachineCloudInstances(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineCloudInstance(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID.String()),
		),
	)
}

// WatchLXDProfiles returns a NotifyWatcher that is subscribed to the changes in
// the machine_cloud_instance table in the model, for the given machine UUID.
// Note: Sometime in the future, this watcher could react to logical changes
// fired from `SetAppliedLXDProfileNames()` instead of the `machine_lxd_profile`
// table, which could become noisy.
func (s *WatchableService) WatchLXDProfiles(ctx context.Context, machineUUID machine.UUID) (watcher.NotifyWatcher, error) {
	_, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineLXDProfiles(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID.String()),
		),
	)
}

// WatchMachineReboot returns a NotifyWatcher that is subscribed to
// the changes in the machine_requires_reboot table in the model.
// It raises an event whenever the machine uuid or its parent is added to the reboot table.
func (s *WatchableService) WatchMachineReboot(ctx context.Context, uuid machine.UUID) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	uuids, err := s.machineToCareForReboot(ctx, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}
	machines := set.NewStrings(transform.Slice(uuids, func(u machine.UUID) string { return u.String() })...)
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineReboot(),
			changestream.All,
			func(s string) bool {
				return machines.Contains(s)
			},
		),
	)
}

func (s *WatchableService) machineToCareForReboot(ctx context.Context, uuid machine.UUID) ([]machine.UUID, error) {
	parentUUID, err := s.st.GetMachineParentUUID(ctx, uuid)
	if err != nil && !errors.Is(err, machineerrors.MachineHasNoParent) {
		return nil, errors.Capture(err)
	}
	if errors.Is(err, machineerrors.MachineHasNoParent) {
		return []machine.UUID{uuid}, nil
	}
	return []machine.UUID{uuid, parentUUID}, nil
}

// uuidToNameMapper is an eventsource.Mapper that converts a slice of
// changestream.ChangeEvent containing machine UUIDs to another slice of
// events with the machine names that correspond to the UUIDs.
// If the input filter is not nil and returns true for any machine UUID/name in
// the events, those events are omitted.
func (s *WatchableService) uuidToNameMapper(filter func(string, machine.Name) bool) eventsource.Mapper {
	return func(
		ctx context.Context, events []changestream.ChangeEvent,
	) ([]string, error) {
		// Generate a slice of UUIDs and placeholders for our query
		// and index the events by those UUIDs.
		machineUUIDs := transform.Slice(events, func(e changestream.ChangeEvent) string {
			return e.Changed()
		})

		uuidsToName, err := s.st.GetNamesForUUIDs(ctx, machineUUIDs)
		if err != nil {
			return nil, errors.Capture(err)
		}

		var changes []string
		for uuid, name := range uuidsToName {
			if filter != nil && filter(uuid, name) {
				continue
			}

			changes = append(changes, name.String())
		}

		return changes, nil
	}
}

// noContainersFilter returns true if the input machine name
// is one reserved for LXD containers.
func noContainersFilter(_ string, name machine.Name) bool {
	return strings.Contains(name.String(), "/")
}
