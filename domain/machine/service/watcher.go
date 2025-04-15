// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/providertracker"
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
) *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{
				st: st,
			},
			providerGetter: providerGetter,
		},
		watcherFactory: watcherFactory,
	}
}

// WatchModelMachines watches for additions or updates to non-container
// machines. It is used by workers that need to factor life value changes,
// and so does not factor machine removals, which are considered to be
// after their transition to the dead state.
// It emits machine names rather than UUIDs.
func (s *WatchableService) WatchModelMachines() (watcher.StringsWatcher, error) {
	table, stmt := s.st.InitialWatchModelMachinesStatement()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		eventsource.InitialNamespaceChanges(stmt),
		uuidToNameMapper(noContainersFilter),
		eventsource.NamespaceFilter(table, changestream.Changed),
	)
}

// WatchMachineCloudInstances returns a NotifyWatcher that is subscribed to
// the changes in the machine_cloud_instance table in the model, for the given
// machine UUID.
func (s *WatchableService) WatchMachineCloudInstances(ctx context.Context, machineUUID string) (watcher.NotifyWatcher, error) {
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineCloudInstance(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID),
		),
	)
}

// WatchLXDProfiles returns a NotifyWatcher that is subscribed to the changes in
// the machine_cloud_instance table in the model, for the given machine UUID.
// Note: Sometime in the future, this watcher could react to logical changes
// fired from `SetAppliedLXDProfileNames()` instead of the `machine_lxd_profile`
// table, which could become noisy.
func (s *WatchableService) WatchLXDProfiles(ctx context.Context, machineUUID string) (watcher.NotifyWatcher, error) {
	return s.watcherFactory.NewNotifyWatcher(
		eventsource.PredicateFilter(
			s.st.NamespaceForWatchMachineLXDProfiles(),
			changestream.All,
			eventsource.EqualsPredicate(machineUUID),
		),
	)
}

// WatchMachineReboot returns a NotifyWatcher that is subscribed to
// the changes in the machine_requires_reboot table in the model.
// It raises an event whenever the machine uuid or its parent is added to the reboot table.
func (s *WatchableService) WatchMachineReboot(ctx context.Context, uuid string) (watcher.NotifyWatcher, error) {
	uuids, err := s.machineToCareForReboot(ctx, uuid)
	if err != nil {
		return nil, errors.Capture(err)
	}
	machines := set.NewStrings(uuids...)
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

func (s *WatchableService) machineToCareForReboot(ctx context.Context, uuid string) ([]string, error) {
	parentUUID, err := s.st.GetMachineParentUUID(ctx, uuid)
	if err != nil && !errors.Is(err, machineerrors.MachineHasNoParent) {
		return nil, errors.Capture(err)
	}
	if errors.Is(err, machineerrors.MachineHasNoParent) {
		return []string{uuid}, nil
	}
	return []string{uuid, parentUUID}, nil
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

// uuidToNameMapper is an eventsource.Mapper that converts a slice of
// changestream.ChangeEvent containing machine UUIDs to another slice of
// events with the machine names that correspond to the UUIDs.
// If the input filter is not nil and returns true for any machine UUID/name in
// the events, those events are omitted.
func uuidToNameMapper(filter func(string, machine.Name) bool) eventsource.Mapper {
	return func(
		ctx context.Context, db database.TxnRunner, events []changestream.ChangeEvent,
	) ([]changestream.ChangeEvent, error) {
		// Generate a slice of UUIDs and placeholders for our query
		// and index the events by those UUIDs.
		machineUUIDs := make([]any, 0, len(events))
		placeHolders := make([]string, 0, len(events))
		eventsByUUID := transform.SliceToMap(events, func(e changestream.ChangeEvent) (string, changestream.ChangeEvent) {
			machineUUIDs = append(machineUUIDs, e.Changed())
			placeHolders = append(placeHolders, "?")
			return e.Changed(), e
		})

		newEvents := make([]changestream.ChangeEvent, 0, len(events))
		q := fmt.Sprintf("SELECT uuid, name FROM machine WHERE uuid IN (%s)", strings.Join(placeHolders, ", "))

		if err := db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
			rows, err := tx.QueryContext(ctx, q, machineUUIDs...)
			if err != nil {
				return err
			}
			defer func() { _ = rows.Close() }()

			var uuid, name string
			for rows.Next() {
				if err := rows.Scan(&uuid, &name); err != nil {
					return err
				}

				if filter != nil && filter(uuid, machine.Name(name)) {
					continue
				}

				e := eventsByUUID[uuid]
				newEvents = append(newEvents, changeEventShim{
					changeType: e.Type(),
					namespace:  e.Namespace(),
					changed:    name,
				})
			}

			return rows.Err()
		}); err != nil {
			return nil, err
		}

		return newEvents, nil
	}
}

// noContainersFilter returns true if the input machine name
// is one reserved for LXD containers.
func noContainersFilter(_ string, name machine.Name) bool {
	return strings.Contains(name.String(), "/")
}
