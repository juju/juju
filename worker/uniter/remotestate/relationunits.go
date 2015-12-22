// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remotestate

import (
	"launchpad.net/tomb"

	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

type relationUnitsWatcher struct {
	tomb       tomb.Tomb
	relationId int
	in         apiwatcher.RelationUnitsWatcher
	out        chan<- relationUnitsChange
}

type relationUnitsChange struct {
	relationId int
	multiwatcher.RelationUnitsChange
}

func newRelationUnitsWatcher(
	relationId int,
	in apiwatcher.RelationUnitsWatcher,
	out chan<- relationUnitsChange,
) *relationUnitsWatcher {
	ruw := &relationUnitsWatcher{relationId: relationId, in: in, out: out}
	go func() {
		defer ruw.tomb.Done()
		// TODO(axw) add Kill() and Wait() to watchers?
		//
		// At the moment we have to rely on the watcher's
		// channel being closed inside loop() to react
		// to it being killed/stopped.
		ruw.tomb.Kill(ruw.loop())
		ruw.tomb.Kill(in.Stop())
	}()
	return ruw
}

func (w *relationUnitsWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

func (w *relationUnitsWatcher) loop() error {
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case change, ok := <-w.in.Changes():
			if !ok {
				return watcher.EnsureErr(w.in)
			}
			select {
			case <-w.tomb.Dying():
				return tomb.ErrDying
			case w.out <- relationUnitsChange{w.relationId, change}:
			}
		}
	}
}
