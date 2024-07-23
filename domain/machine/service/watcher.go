// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

type WatchableService struct {
	Service
	watcherFactory WatcherFactory
}

// WatcherFactory describes methods for creating watchers.
type WatcherFactory interface {
	// NewNamespaceWatcher returns a new namespace watcher
	// for events based on the input change mask.
	NewNamespaceWatcher(
		namespace string, changeMask changestream.ChangeType, initialStateQuery eventsource.NamespaceQuery,
	) (watcher.StringsWatcher, error)

	NewNamespaceMapperWatcher(
		namespace string,
		changeMask changestream.ChangeType,
		initialStateQuery eventsource.NamespaceQuery,
		mapper eventsource.Mapper,
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

// WatchModelMachines watches for additions or updates to non-container
// machines. It is used by workers that need to factor life value changes,
// and so does not factor machine removals, which are considered to be
// after their transition to the dead state.
// It emits machine names rather than UUIDs.
func (s *WatchableService) WatchModelMachines() (watcher.StringsWatcher, error) {
	table, stmt := s.st.InitialWatchModelMachinesStatement()
	return s.watcherFactory.NewNamespaceMapperWatcher(
		table,
		changestream.Create|changestream.Update,
		eventsource.InitialNamespaceChanges(stmt),
		uuidToNameMapper(noContainersFilter),
	)
}

// WatchMachineCloudInstances returns a StringsWatcher that is subscribed to
// the changes in the machine_cloud_instance table in the model.
func (s *WatchableService) WatchMachineCloudInstances(ctx context.Context) (watcher.StringsWatcher, error) {
	table, stmt := s.st.InitialWatchInstanceStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		table,
		changestream.All,
		eventsource.InitialNamespaceChanges(stmt),
	)
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
