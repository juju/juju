// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/watcher"
)

type wrappedRelationUnitsWatcher struct {
	catacomb   catacomb.Catacomb
	relationId int
	changes    watcher.RelationUnitsChannel
	out        chan<- relationUnitsChange
}

type relationUnitsChange struct {
	relationId int
	watcher.RelationUnitsChange
}

// wrapRelationUnitsWatcher creates a new worker that takes values from the
// supplied watcher's Changes chan, annotates them with the supplied relation
// id, and delivers then on the supplied out chan.
//
// The caller releases responsibility for stopping the supplied watcher and
// waiting for errors, *whether or not this method succeeds*.
func wrapRelationUnitsWatcher(
	relationId int,
	watcher watcher.RelationUnitsWatcher,
	out chan<- relationUnitsChange,
) (*wrappedRelationUnitsWatcher, error) {
	ruw := &wrappedRelationUnitsWatcher{
		relationId: relationId,
		changes:    watcher.Changes(),
		out:        out,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units-watcher",
		Site: &ruw.catacomb,
		Work: ruw.loop,
		Init: []worker.Worker{watcher},
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return ruw, nil
}

// Kill is part of the worker.Worker interface.
func (w *wrappedRelationUnitsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *wrappedRelationUnitsWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *wrappedRelationUnitsWatcher) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.changes:
			if !ok {
				return errors.New("watcher closed channel")
			}
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.out <- relationUnitsChange{w.relationId, change}:
			}
		}
	}
}
