// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/collections/set"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher/eventsource"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/errors"
)

// LifeGetter is a function which looks up life values of the entities with the specified IDs.
type LifeGetter func(ctx context.Context, ids []string) (map[string]life.Life, error)

// LifeStringsWatcherMapperFunc returns a namespace watcher mapper function which emits
// events when the life of an entity changes. The supplied lifeGetter func is used to
// retrieve a map of life values, keyed on IDs which must match those supplied by the
// source event stream.
// The source event stream may supply ids the caller is not interested in. These are
// filtered out after loading the current values from state.
func LifeStringsWatcherMapperFunc(logger logger.Logger, lifeGetter LifeGetter) eventsource.Mapper {
	knownLife := make(map[string]life.Life)

	return func(ctx context.Context, changes []changestream.ChangeEvent) (_ []string, err error) {
		defer func() {
			if err != nil {
				logger.Errorf(ctx, "running life watcher mapper func: %v", err)
			}
		}()

		events := make(map[string]changestream.ChangeEvent, len(changes))

		// Extract the ids of the changed entities.
		ids := set.NewStrings()
		for _, change := range changes {
			events[change.Changed()] = change
			ids.Add(change.Changed())
		}
		logger.Debugf(ctx, "got changes for ids: %v", ids.Values())

		// First record any deleted entities and remove from the
		// set of ids we are interested in looking up the life for.
		latest := make(map[string]life.Life)
		for _, change := range events {
			if change.Type() == changestream.Deleted {
				latest[change.Changed()] = life.Dead
				ids.Remove(change.Changed())
			}
		}

		// Separate ids into those thought to exist and those known to be removed.
		// Gather the latest life values of the ids.
		currentValues, err := lifeGetter(ctx, ids.Values())
		if err != nil {
			return nil, errors.Capture(err)
		}

		// We queried the ids that were not removed. The result contains
		// only those we're interested in, so any extra needs to be
		// removed from subsequent processing.
		unknownIDs := set.NewStrings(ids.Values()...)
		for id, l := range currentValues {
			unknownIDs.Remove(id)
			latest[id] = l
		}
		logger.Debugf(ctx, "ignoring unknown ids %v", unknownIDs.Values())

		for _, id := range unknownIDs.Values() {
			delete(latest, id)
			delete(events, id)
		}

		logger.Debugf(ctx, "processing latest life values for %v", latest)

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

		// Preserve the order of the changes as they were received, but
		// filter out any that are not in the knownLife map.
		var result []string
		for _, change := range changes {
			if _, ok := events[change.Changed()]; !ok {
				continue
			}

			result = append(result, change.Changed())
		}
		return result, nil
	}
}
