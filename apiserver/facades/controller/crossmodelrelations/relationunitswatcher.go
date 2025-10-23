// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// RelationUnitsWatcherService defines the interface required to watch for local relation changes.
type RelationUnitsWatcherService interface {
	// GetConsumerRelationUnitsChange returns the versions of the relation units
	// settings and any departed units.
	GetConsumerRelationUnitsChange(
		context.Context,
		corerelation.UUID,
		coreapplication.UUID,
	) (relation.ConsumerRelationUnitsChange, error)
}

// RelationChangesWatcher represents a relation.RelationUnitsWatcher at the
// apiserver level (different type for changes).
type RelationChangesWatcher = watcher.Watcher[params.RelationUnitsChange]

// wrappedRelationUnitsWatcher wraps a domain level NotifyWatcher from
// WatchRelationUnits in an apiserver level one, taking responsibility
// for the source watcher's lifetime.
func wrappedRelationChangesWatcher(
	source watcher.NotifyWatcher,
	offerApplicationUUID coreapplication.UUID,
	offerRelationUUID corerelation.UUID,
	service RelationUnitsWatcherService,
) (RelationChangesWatcher, error) {
	w := &relationChangesWatcher{
		source:               source,
		out:                  make(chan params.RelationUnitsChange),
		offerApplicationUUID: offerApplicationUUID,
		offerRelationUUID:    offerRelationUUID,
		service:              service,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units-watcher",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{source},
	})
	return w, errors.Capture(err)
}

type relationChangesWatcher struct {
	source   watcher.NotifyWatcher
	out      chan params.RelationUnitsChange
	catacomb catacomb.Catacomb
	service  RelationUnitsWatcherService

	offerApplicationUUID coreapplication.UUID
	offerRelationUUID    corerelation.UUID
	data                 relation.ConsumerRelationUnitsChange
}

// Changes is part of RelationUnitsWatcher.
func (w *relationChangesWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.out
}

// Kill is part of worker.Worker.
func (w *relationChangesWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of worker.Worker.
func (w *relationChangesWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *relationChangesWatcher) loop() error {
	ctx := w.catacomb.Context(context.Background())

	data, err := w.service.GetConsumerRelationUnitsChange(ctx, w.offerRelationUUID, w.offerApplicationUUID)
	if err != nil {
		return errors.Errorf("fetching consumer side of relation %v: %w", w.offerRelationUUID, err)
	}
	w.data = data

	// We need to close the changes channel because we're inside the
	// API - see apiserver/watcher.go:srvRelationUnitsWatcher.Next()
	defer close(w.out)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case _, ok := <-w.source.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("relation units watcher closed")
				}
			}

			unitRelationInfo, err := w.service.GetConsumerRelationUnitsChange(ctx, w.offerRelationUUID, w.offerApplicationUUID)
			if err != nil {
				return errors.Errorf("fetching consumer side of relation %v: %w", w.offerRelationUUID, err)
			}

			info, sendEvent := w.convert(unitRelationInfo)
			if !sendEvent {
				continue
			}

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- info:
			}
		}
	}
}

func (w *relationChangesWatcher) convert(
	newChange relation.ConsumerRelationUnitsChange,
) (params.RelationUnitsChange, bool) {
	changes := w.dataChanges(newChange)
	if changes.Empty() {
		return params.RelationUnitsChange{}, false
	}

	// If there are changes, keep the latest version for next time.
	w.data = newChange

	unitsChanged := transform.Map(changes.UnitsSettingsVersions, func(key string, val int64) (string, params.UnitSettings) {
		return key, params.UnitSettings{Version: val}
	})

	return params.RelationUnitsChange{
		Changed:    unitsChanged,
		AppChanged: changes.AppSettingsVersion,
		Departed:   changes.DepartedUnits,
	}, true
}

// dataChanges returns the changed data since the last time this watcher
// was triggered.
func (w *relationChangesWatcher) dataChanges(event relation.ConsumerRelationUnitsChange) relation.ConsumerRelationUnitsChange {
	changedEvent := relation.ConsumerRelationUnitsChange{}

	departed := set.NewStrings(event.DepartedUnits...).Difference(set.NewStrings(w.data.DepartedUnits...))
	if !departed.IsEmpty() {
		changedEvent.DepartedUnits = departed.SortedValues()
	}

	appChanges := w.mapDifference(w.data.AppSettingsVersion, event.AppSettingsVersion)
	if len(appChanges) > 0 {
		changedEvent.AppSettingsVersion = appChanges
	}

	unitChanges := w.mapDifference(w.data.UnitsSettingsVersions, event.UnitsSettingsVersions)
	if len(unitChanges) > 0 {
		changedEvent.UnitsSettingsVersions = unitChanges
	}

	return changedEvent
}

func (w *relationChangesWatcher) mapDifference(oldMap, newMap map[string]int64) map[string]int64 {
	difference := make(map[string]int64, 0)
	for k, newV := range newMap {
		oldV, ok := oldMap[k]
		if !ok || oldV != newV {
			difference[k] = newV
		}
	}
	return difference
}
