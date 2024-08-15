// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"

	"github.com/juju/juju/core/changestream"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
)

// LifeGetter is a function which looks up life values of the entities with the specified IDs.
type LifeGetter func(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]life.Life, error)

// LifeStringsWatcherMapperFunc returns a namespace watcher mapper function which emits
// events when the life of an entity changes. The supplied lifeGetter func is used to
// retrieve a map of life values, keyed on IDs which must match those supplied by the
// source event stream. If discriminator is supplied, it is used to filter out unwanted events.
func LifeStringsWatcherMapperFunc(lifeGetter LifeGetter, discriminator ...string) eventsource.Mapper {
	knownLife := make(map[string]life.Life)

	return func(ctx context.Context, db coredatabase.TxnRunner, changes []changestream.ChangeEvent) ([]changestream.ChangeEvent, error) {
		events := make(map[string]changestream.ChangeEvent, len(changes))

		// Filter out any events that don't match the discriminator.
		ids := set.NewStrings()
		for _, change := range changes {
			if len(discriminator) > 0 && discriminator[0] != change.Discriminator() {
				continue
			}
			events[change.Changed()] = change
			ids.Add(change.Changed())
		}

		// Separate ids into those thought to exist and those known to be removed.
		latest := make(map[string]life.Life)
		for _, change := range changes {
			if change.Type() == changestream.Delete {
				latest[change.Changed()] = life.Dead
				ids.Remove(change.Changed())
				continue
			}
		}

		// Gather the latest life values of the ids.
		currentValues, err := lifeGetter(ctx, db, ids.Values())
		if err != nil {
			return nil, errors.Trace(err)
		}
		for id, l := range currentValues {
			latest[id] = l
		}

		// Add to ids any whose life state is known to have changed.
		for id, newLife := range latest {
			gone := newLife == life.Dead
			oldLife, known := knownLife[id]
			switch {
			case known && gone:
				delete(knownLife, id)
			case !known && !gone:
				knownLife[id] = newLife
			case known && newLife != oldLife:
				knownLife[id] = newLife
			default:
				delete(events, id)
			}
		}
		var result []changestream.ChangeEvent
		for _, e := range events {
			result = append(result, e)
		}
		return result, nil
	}
}
