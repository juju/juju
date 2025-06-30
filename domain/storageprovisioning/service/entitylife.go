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
// This func expect to be provided with a func that provides the initial life
// values to start performing change detection on top of. This func will be
// called once on the first invocation of the returned mapper.
//
// The returned mapper is not thread safe.
func EntityLifeMapperFunc(
	initialLife EntityLifeGetter,
	lifeGetter EntityLifeGetter,
) eventsource.Mapper {
	var haveInitialLife bool
	var knownLife map[string]life.Life
	return func(
		ctx context.Context, _ []changestream.ChangeEvent,
	) ([]string, error) {
		if !haveInitialLife {
			l, err := initialLife(ctx)
			if err != nil {
				return nil, errors.Errorf("getting initial life for mapper: %w", err)
			}
			haveInitialLife = true
			knownLife = l
		}

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
		// Append all the entities that have been removed which knownLife now
		// embodies.
		changes = slices.AppendSeq(changes, maps.Keys(knownLife))

		// Reset knownLife
		knownLife = latestLife
		return changes, nil
	}
}

// MakeEntityLifePrerequisites is a helper function for establishing the
// prerequisites required for watching the life of storage entities associated
// with a concern. A concern in this case is a machine being watched by a
// provisioner.
//
// This helper function exists to make sure that the initial life values
// reported by the initial query are used by the returned mapper for detecting
// change. Keeping the initial query and the mapper in sync stops the
// introduction of race conditions when reporting changes.
//
// If the initial query has not provided any initial values to the mapper then
// the mapper starts from an empty set of known life values. The design is done
// like this so that the two components never create a dead lock between one
// another.
//
// This function returns a new initial query that can be supplied to the watcher
// for seeding a set of initial values.
func MakeEntityLifePrerequisites(
	initialQuery eventsource.Query[map[string]life.Life],
	lifeGetter EntityLifeGetter,
) (eventsource.NamespaceQuery, eventsource.Mapper) {
	// Make a buffered channel to capture the initial query values.
	// Shimmed query is responsible for closing the channel.
	initial := make(chan map[string]life.Life, 1)
	shimmedInitialQuery := func(ctx context.Context, db database.TxnRunner) ([]string, error) {
		defer close(initial)
		initQueryData, err := initialQuery(ctx, db)
		if err != nil {
			return nil, errors.Errorf(
				"running initial query for entity life values: %w", err,
			)
		}
		select {
		case initial <- initQueryData:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return slices.Collect(maps.Keys(initQueryData)), nil
	}

	initialLifeForMapper := func(ctx context.Context) (map[string]life.Life, error) {
		select {
		case initialData, ok := <-initial:
			if !ok {
				return nil, errors.New(
					"initial life can only be called once or initial query failed",
				)
			}
			return initialData, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
			return nil, nil
		}
	}

	return shimmedInitialQuery, EntityLifeMapperFunc(
		initialLifeForMapper, lifeGetter)
}
