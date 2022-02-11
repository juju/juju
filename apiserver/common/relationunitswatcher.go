// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// RelationUnitsWatcher represents a state.RelationUnitsWatcher at the
// apiserver level (different type for changes).
type RelationUnitsWatcher interface {
	watcher.CoreWatcher
	Changes() <-chan params.RelationUnitsChange

	// Stop is needed to implement facade.Resource.
	Stop() error

	// Err implements watcher.Errer.
	Err() error
}

// RelationUnitsWatcherFromState wraps a state-level
// RelationUnitsWatcher in an equivalent apiserver-level one, taking
// responsibility for the source watcher's lifetime.
func RelationUnitsWatcherFromState(source state.RelationUnitsWatcher) (RelationUnitsWatcher, error) {
	w := &relationUnitsWatcher{
		source: source,
		out:    make(chan params.RelationUnitsChange),
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{source},
	})
	return w, errors.Trace(err)
}

type relationUnitsWatcher struct {
	source   state.RelationUnitsWatcher
	out      chan params.RelationUnitsChange
	catacomb catacomb.Catacomb
}

func (w *relationUnitsWatcher) loop() error {
	// We need to close the changes channel because we're inside the
	// API - see apiserver/watcher.go:srvRelationUnitsWatcher.Next()
	defer close(w.out)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case event, ok := <-w.source.Changes():
			if !ok {
				return w.catacomb.ErrDying()
			}
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- w.convert(event):
			}
		}
	}
}

func (w *relationUnitsWatcher) convert(
	event watcher.RelationUnitsChange,
) params.RelationUnitsChange {
	var changed map[string]params.UnitSettings
	if event.Changed != nil {
		changed = make(map[string]params.UnitSettings, len(event.Changed))
		for key, val := range event.Changed {
			changed[key] = params.UnitSettings{Version: val.Version}
		}
	}
	return params.RelationUnitsChange{
		Changed:    changed,
		AppChanged: event.AppChanged,
		Departed:   event.Departed,
	}
}

// Changes is part of RelationUnitsWatcher.
func (w *relationUnitsWatcher) Changes() <-chan params.RelationUnitsChange {
	return w.out
}

// Kill is part of worker.Worker.
func (w *relationUnitsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of worker.Worker.
func (w *relationUnitsWatcher) Wait() error {
	return w.catacomb.Wait()
}

// Stop is part of facade.Resource.
func (w *relationUnitsWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

// Err is part of state/watcher.Errer.
func (w *relationUnitsWatcher) Err() error {
	return w.catacomb.Err()
}
