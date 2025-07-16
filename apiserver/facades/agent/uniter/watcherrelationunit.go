// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"

	"github.com/juju/collections/transform"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/core/application"
	corerelation "github.com/juju/juju/core/relation"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/relation"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// relationUnitsWatcher watches changes in related units for a specific unit
// and relation.  It handles lifecycle management and manages buffers for
// internal event streaming, working as a proxy around the domain watcher to
// enrich its events.
type relationUnitsWatcher struct {
	catacomb catacomb.Catacomb

	relation RelationService

	unitUUID     coreunit.UUID
	relationUUID corerelation.UUID

	out chan params.RelationUnitsChange
}

// newRelationUnitsWatcher creates and starts a watcher for observing changes
// in relation units for a given unit and relation. Changes are generated
// whenever any related units or application (ie, unit or application that are
// bound to this unit through this relation):
// - has its settings updated
// - enter or leave scope (for related units)
//
// It initializes a relationUnitsWatcher instance,
// sets up its internal state, and starts its lifecycle management.
func newRelationUnitsWatcher(
	unitUUID coreunit.UUID, relUUID corerelation.UUID, relationService RelationService,
) (common.RelationUnitsWatcher, error) {
	w := &relationUnitsWatcher{
		relation:     relationService,
		unitUUID:     unitUUID,
		relationUUID: relUUID,
		out:          make(chan params.RelationUnitsChange),
	}
	return w, catacomb.Invoke(catacomb.Plan{
		Name: "relation-units-watcher",
		Site: &w.catacomb,
		Work: func() error {
			return w.loop()
		},
	})
}

// fetchRelationUnitChanges processes a list of changes from domain watcher,
// categorizing them by type, and retrieves the updated relation unit data.
func (w *relationUnitsWatcher) fetchRelationUnitChanges(ctx context.Context,
	changes []string) (params.RelationUnitsChange, error) {
	var changedUnitUUIDs []coreunit.UUID
	var changedAppUUIDs []application.ID

	// sort uuid by kind
	for _, change := range changes {
		kind, uuid, err := relation.DecodeWatchRelationUnitChangeUUID(change)
		if err != nil {
			return params.RelationUnitsChange{}, internalerrors.Capture(err)
		}
		switch kind {
		case relation.UnitUUID:
			changedUnitUUIDs = append(changedUnitUUIDs, coreunit.UUID(uuid))
		case relation.ApplicationUUID:
			changedAppUUIDs = append(changedAppUUIDs, application.ID(uuid))
		default:
			return params.RelationUnitsChange{}, internalerrors.Errorf("unknown relation unit change kind: %q", kind)
		}
	}

	fetched, err := w.relation.GetRelationUnitChanges(ctx, changedUnitUUIDs, changedAppUUIDs)
	if err != nil {
		return params.RelationUnitsChange{}, internalerrors.Errorf("fetching related units watcher changes: %w", err)
	}

	return convertRelationUnitsChange(fetched), nil
}

func convertRelationUnitsChange(changes relation.RelationUnitsChange) params.RelationUnitsChange {
	var changed map[string]params.UnitSettings
	if changes.Changed != nil {
		changed = make(map[string]params.UnitSettings, len(changes.Changed))
		for key, val := range changes.Changed {
			changed[key.String()] = params.UnitSettings{Version: val}
		}
	}
	return params.RelationUnitsChange{
		Changed: transform.Map(changes.Changed, func(k coreunit.Name, v int64) (string, params.UnitSettings) {
			return k.String(), params.UnitSettings{Version: v}
		}),
		AppChanged: changes.AppChanged,
		Departed:   transform.Slice(changes.Departed, coreunit.Name.String),
	}
}

// loop manages the lifecycle of the relationUnitsWatcher, processes related
// unit changes, and outputs them to a channel.
func (w *relationUnitsWatcher) loop() error {
	defer close(w.out)

	ctx := w.catacomb.Context(context.Background())

	domainWatcher, err := w.relation.WatchRelatedUnits(ctx, w.unitUUID, w.relationUUID)
	if err != nil {
		return internalerrors.Errorf("starting related units watcher for relation %q and unit %q: %w",
			w.relationUUID, w.unitUUID, err)
	}
	if err := w.catacomb.Add(domainWatcher); err != nil {
		return internalerrors.Errorf("adding related units watcher to catacomb: %w", err)
	}

	var change params.RelationUnitsChange
	var out chan params.RelationUnitsChange
	in := domainWatcher.Changes()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case changes, ok := <-in:
			if !ok {
				return w.catacomb.ErrDying()
			}
			change, err = w.fetchRelationUnitChanges(ctx, changes)
			if err != nil {
				return internalerrors.Errorf("fetching related units watcher changes: %w", err)
			}
			in, out = nil, w.out
		case out <- change:
			in, out = domainWatcher.Changes(), nil
		}
	}
}

// Kill is an implementation of [watcher.RelationUnitsWatcher]
func (w *relationUnitsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is an implementation of [watcher.RelationUnitsWatcher]
func (w *relationUnitsWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Changes is an implementation of [watcher.RelationUnitsWatcher]
func (w *relationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.out
}

var _ common.RelationUnitsWatcher = &relationUnitsWatcher{}
