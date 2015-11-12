// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"github.com/juju/errors"

	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker"
	"github.com/juju/juju/worker/catacomb"
)

type relationUnitsWatcher struct {
	catacomb   catacomb.Catacomb
	relationId int
	watcher    watcher.RelationUnitsWatcher
	out        chan<- relationUnitsChange
}

type relationUnitsChange struct {
	relationId int
	watcher.RelationUnitsChange
}

// newRelationUnitsWatcher creates a new worker that takes values from the
// supplied watcher's Changes chan, annotates them with the supplied relation
// id, and delivers then on the supplied out chan.
//
// The caller releases responsibility for stopping the supplied watcher and
// waiting for errors, *whether or not this method succeeds*.
func newRelationUnitsWatcher(
	relationId int,
	watcher watcher.RelationUnitsWatcher,
	out chan<- relationUnitsChange,
) (*relationUnitsWatcher, error) {
	ruw := &relationUnitsWatcher{
		relationId: relationId,
		watcher:    watcher,
		out:        out,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &ruw.catacomb,
		Work: ruw.loop,
	})
	if err != nil {
		if stopErr := worker.Stop(watcher); err != nil {
			logger.Errorf("while stopping relation units watcher: %v", stopErr)
		}
		return nil, errors.Trace(err)
	}
	return ruw, nil
}

// Kill is part of the worker.Worker interface.
func (w *relationUnitsWatcher) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is part of the worker.Worker interface.
func (w *relationUnitsWatcher) Wait() error {
	return w.catacomb.Wait()
}

func (w *relationUnitsWatcher) loop() error {
	if err := w.catacomb.Add(w.watcher); err != nil {
		return err
	}
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.watcher.Changes():
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
	return nil
}
