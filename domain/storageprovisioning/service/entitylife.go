// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"maps"
	"slices"

	"github.com/juju/juju/core/changestream"
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

// EntityLifeMapperFuc provides a watcher mapper that can be used to
// take change events for one concern and translate this into a set of entity
// life changes. Entity in this case is a storage entity that is associated
// with the context of the current cocern.
//
// A concrete example would be a machine provisioner concern where the concern
// is the net node of the machine being watched. Entities are all the storage
// entities that would be provisioned from this net node context.
//
// When an entities life has change compared with the last seen value or an
// entity has been added or removed will result in the id being returned in the
// change set.
//
// [EntityLifeGetter] provides the latest life values for the concerns entities.
func EntityLifeMapperFuc(
	ctx context.Context, lifeGetter EntityLifeGetter,
) (eventsource.Mapper, error) {
	knownLife, err := lifeGetter(ctx)
	if err != nil {
		return nil, errors.Errorf(
			"getting initial storage entity known life values: %w", err,
		)
	}

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
			if v != knownLife[k] {
				changes = append(changes, k)
			}
			delete(knownLife, k)
		}
		changes = slices.AppendSeq(changes, maps.Keys(knownLife))
		knownLife = latestLife
		return changes, nil
	}, nil
}
