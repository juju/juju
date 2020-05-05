// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/catacomb"
	"gopkg.in/macaroon-bakery.v2/bakery"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names/v4"
)

// relationUnitsWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type relationUnitsWorker struct {
	catacomb    catacomb.Catacomb
	relationTag names.RelationTag
	rrw         watcher.RemoteRelationWatcher
	changes     chan<- params.RemoteRelationChangeEvent
	macaroon    *macaroon.Macaroon

	logger Logger
}

func newRelationUnitsWorker(
	relationTag names.RelationTag,
	macaroon *macaroon.Macaroon,
	rrw watcher.RemoteRelationWatcher,
	changes chan<- params.RemoteRelationChangeEvent,
	logger Logger,
) (*relationUnitsWorker, error) {
	w := &relationUnitsWorker{
		relationTag: relationTag,
		macaroon:    macaroon,
		rrw:         rrw,
		changes:     changes,
		logger:      logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{rrw},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *relationUnitsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *relationUnitsWorker) Wait() error {
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf("error in relation units worker for %v: %v", w.relationTag.Id(), err)
	}
	return err
}

func (w *relationUnitsWorker) loop() error {
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.rrw.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			w.logger.Debugf("relation units changed for %v: %#v", w.relationTag, change)
			if isEmpty(change) {
				continue
			}

			// Add macaroon in case this event is sent to a remote
			// facade.

			// TODO(babbageclunk): move this so it happens just before
			// the event is published to the remote facade.
			change.Macaroons = macaroon.Slice{w.macaroon}
			change.BakeryVersion = bakery.LatestVersion

			// Send in lockstep so we don't drop events (otherwise
			// we'd need to merge them - not too hard in this
			// case but probably not needed).
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- change:
			}
		}
	}
}

func isEmpty(change params.RemoteRelationChangeEvent) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}
