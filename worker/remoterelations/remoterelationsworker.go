// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"gopkg.in/juju/names.v2"
	"gopkg.in/juju/worker.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/juju/worker/catacomb"
)

// remoteRelationsWorker listens for changes to the
// life and status of a relation in the offering model.
type remoteRelationsWorker struct {
	catacomb catacomb.Catacomb

	relationTag         names.RelationTag
	remoteRelationToken string
	applicationToken    string
	relationsWatcher    watcher.RelationStatusWatcher
	changes             chan<- params.RemoteRelationChangeEvent
}

func newRemoteRelationsWorker(
	relationTag names.RelationTag,
	applicationToken string,
	remoteRelationToken string,
	relationsWatcher watcher.RelationStatusWatcher,
	changes chan<- params.RemoteRelationChangeEvent,
) (*remoteRelationsWorker, error) {
	w := &remoteRelationsWorker{
		relationsWatcher:    relationsWatcher,
		relationTag:         relationTag,
		remoteRelationToken: remoteRelationToken,
		applicationToken:    applicationToken,
		changes:             changes,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{relationsWatcher},
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *remoteRelationsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *remoteRelationsWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteRelationsWorker) loop() error {
	var (
		changes chan<- params.RemoteRelationChangeEvent
		event   params.RemoteRelationChangeEvent
	)
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case relChanges, ok := <-w.relationsWatcher.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			if len(relChanges) == 0 {
				logger.Warningf("relation status watcher event with no changes")
				continue
			}
			// We only care about the most recent change.
			change := relChanges[len(relChanges)-1]
			logger.Debugf("relation status changed for %v: %v", w.relationTag, change)
			suspended := change.Suspended
			event = params.RemoteRelationChangeEvent{
				RelationToken:    w.remoteRelationToken,
				ApplicationToken: w.applicationToken,
				Life:             params.Life(change.Life),
				Suspended:        &suspended,
				SuspendedReason:  change.SuspendedReason,
			}
			changes = w.changes

		case changes <- event:
			changes = nil
		}
	}
}
