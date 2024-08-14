// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package watcher

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
)

// lifeStringsWatcher is a watcher that watches for changes
// to the life of a particular set of entities.
type lifeStringsWatcher struct {
	catacomb catacomb.Catacomb
	logger   logger.Logger

	sourceWatcher StringsWatcher
	lifeGetter    func(ctx context.Context, ids []string) (map[string]life.Value, error)

	out chan []string

	// life holds the most recent known life states of interesting entities.
	life map[string]life.Value
}

func NewLifeStringsWatcher(
	sourceWatcher StringsWatcher, logger logger.Logger,
	lifeGetter func(ctx context.Context, ids []string) (map[string]life.Value, error),
) (*lifeStringsWatcher, error) {
	w := &lifeStringsWatcher{
		sourceWatcher: sourceWatcher,
		logger:        logger,
		lifeGetter:    lifeGetter,
		out:           make(chan []string),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{sourceWatcher},
	})
	return w, errors.Trace(err)
}

func (w *lifeStringsWatcher) scopedContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}

func (w *lifeStringsWatcher) initial() ([]string, error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	values, err := w.lifeGetter(ctx, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	var result []string
	for id, l := range values {
		if l != life.Dead {
			w.life[id] = l
		}
		result = append(result, id)
	}
	return result, nil
}

func (w *lifeStringsWatcher) merge(previousChanges []string, events ...string) ([]string, error) {
	ctx, cancel := w.scopedContext()
	defer cancel()

	result := set.NewStrings(previousChanges...)

	// Separate ids into those thought to exist and those known to be removed.
	latest := make(map[string]life.Value)
	orphanedIds := set.NewStrings(events...)

	// Collect life states from ids thought to exist. Any that don't actually
	// exist are ignored (we'll hear about them in the next set of updates --
	// all that's actually happened in that situation is that the watcher
	// events have lagged a little behind reality).
	newValues, err := w.lifeGetter(ctx, events)
	if err != nil {
		return nil, errors.Trace(err)
	}

	for id, l := range newValues {
		orphanedIds.Remove(id)
		latest[id] = l
	}
	for id := range orphanedIds {
		latest[id] = life.Dead
	}

	// Add to ids any whose life state is known to have changed.
	for id, newLife := range latest {
		gone := newLife == life.Dead
		oldLife, known := w.life[id]
		switch {
		case known && gone:
			delete(w.life, id)
		case !known && !gone:
			w.life[id] = newLife
		case known && newLife != oldLife:
			w.life[id] = newLife
		default:
			continue
		}
		result.Add(id)
	}
	return result.SortedValues(), nil
}

func (w *lifeStringsWatcher) loop() error {
	defer close(w.out)

	var (
		changes []string
		err     error
	)
	changes, err = w.initial()
	if err != nil {
		return err
	}
	out := w.out

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case events, ok := <-w.sourceWatcher.Changes():
			if !ok {
				return errors.Errorf("event watcher closed")
			}
			changes, err = w.merge(changes, events...)
			if err != nil {
				return errors.Trace(err)
			}
			if len(changes) > 0 {
				out = w.out
			}
		case out <- changes:
			changes = nil
			out = nil
		}
	}
}

// Changes returns the channel of changes.
func (w *lifeStringsWatcher) Changes() <-chan []string {
	return w.out
}

// Stop stops the watcher.
func (w *lifeStringsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Kill kills the watcher via its tomb.
func (w *lifeStringsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the watcher's tomb to die,
// and returns the error with which it was killed.
func (w *lifeStringsWatcher) Wait() error {
	return w.catacomb.Wait()
}
