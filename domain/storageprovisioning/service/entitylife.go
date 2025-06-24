// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// EntityLifeGetter is a function which returns the current life for each
// concerned entity involved in the context. The string id value used for
// entities must be consistent across multiple calls.
//
// The purpose of this function is to help work out what storage entities that
// a machine provisioner cares about have had a life change.
type EntityLifeGetter func(context.Context) (map[string]life.Life, error)

// EntityLifeInitialQuery returns an initial entity life query based off of the
// initial life values provided. The [eventsource.NamespaceQuery] returned will
// not use the context nor the database transaction runner provided. The query
// just returns the pre calculated initial values from the initial life map.
//
// Upon returning from this function no further use of the map provided as
// initial life will take place. This makes the resultant query safe across go
// routines.
func EntityLifeInitialQuery(
	ctx context.Context,
	initialLife map[string]life.Life,
) eventsource.NamespaceQuery {
	s := slices.AppendSeq(
		make([]string, 0, len(initialLife)),
		maps.Keys(initialLife),
	)
	return func(_ context.Context, _ database.TxnRunner) ([]string, error) {
		return s, nil
	}
}

// EntityLifeMapperFunc provides a watcher mapper that can be used to
// take change events for one concern and translate this into a set of entity
// life changes. Entity in this case is a storage entity that is associated
// with the context of the current cocern.
//
// A concrete example would be a machine provisioner concern where the concern
// is the net node of the machine being watched. Entities are all the storage
// entities that would be provisioned from this net node context.
//
// When an entities life has change compared with the last seen value or an
// entity has been added or removed it will result in the id being returned in
// the change set.
//
// [EntityLifeGetter] provides the latest life values for the concerns entities.
// This func expects to be seeded with the initial life values to use when
// working out change. This is done so that any external reporters of initial
// state can be in sync with the mapper.
func EntityLifeMapperFunc(
	knownLife map[string]life.Life,
	lifeGetter EntityLifeGetter,
) eventsource.Mapper {
	return func(
		ctx context.Context, _ []changestream.ChangeEvent,
	) ([]string, error) {
		latestLife, err := lifeGetter(ctx)
		if err != nil {
			return nil, errors.Errorf(
				"getting latest storage entity life values: %w", err,
			)
		}

		changes := []string(nil)
		for k, v := range latestLife {
			if l, has := knownLife[k]; !has || v != l {
				changes = append(changes, k)
			}
			delete(knownLife, k)
		}
		changes = slices.AppendSeq(changes, maps.Keys(knownLife))
		knownLife = latestLife
		return changes, nil
	}
}

// MakeEntityLifePrerequisites is a helper function for establishing the
// prerequisites required for watching the life of storage entities associated
// against a concern. A concern in this case is a machine being watched by a
// provisioner.
//
// This helper function exists to make sure that the initial life values
// reported by the initial query is kept in sync with the knowledge used in the
// mapper. Without this being done there does exist a small posability of a
// watcher missing an event and expected state not becoming eventually
// consistent.
func MakeEntityLifePrerequisites(
	ctx context.Context,
	lifeGetter EntityLifeGetter,
) (eventsource.NamespaceQuery, eventsource.Mapper, error) {
	initialLife, err := lifeGetter(ctx)
	if err != nil {
		return nil, nil, errors.Errorf(
			"getting initial entity life state from life getter: %w", err,
		)
	}

	namespaceQuery := EntityLifeInitialQuery(ctx, initialLife)
	mapper := EntityLifeMapperFunc(initialLife, lifeGetter)
	return namespaceQuery, mapper, nil
}
