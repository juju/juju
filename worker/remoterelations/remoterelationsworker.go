// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/worker/v3"
	"github.com/juju/worker/v3/catacomb"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/names/v4"
)

// remoteRelationsWorker listens for changes to the
// life and status of a relation in the offering model.
type remoteRelationsWorker struct {
	catacomb catacomb.Catacomb

	relationTag         names.RelationTag
	remoteRelationToken string
	applicationToken    string
	relationsWatcher    watcher.RelationStatusWatcher
	changes             chan<- RelationUnitChangeEvent
	logger              Logger
}

func newRemoteRelationsWorker(
	relationTag names.RelationTag,
	applicationToken string,
	remoteRelationToken string,
	relationsWatcher watcher.RelationStatusWatcher,
	changes chan<- RelationUnitChangeEvent,
	logger Logger,
) (*remoteRelationsWorker, error) {
	w := &remoteRelationsWorker{
		relationsWatcher:    relationsWatcher,
		relationTag:         relationTag,
		remoteRelationToken: remoteRelationToken,
		applicationToken:    applicationToken,
		changes:             changes,
		logger:              logger,
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
	err := w.catacomb.Wait()
	if err != nil {
		w.logger.Errorf("error in remote relations worker for relation %v: %v", w.relationTag.Id(), err)
	}
	return err
}

func (w *remoteRelationsWorker) loop() error {
	var (
		changes chan<- RelationUnitChangeEvent
		event   RelationUnitChangeEvent
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
				w.logger.Warningf("relation status watcher event with no changes")
				continue
			}
			// We only care about the most recent change.
			change := relChanges[len(relChanges)-1]
			w.logger.Debugf("relation status changed for %v: %v", w.relationTag, change)
			suspended := change.Suspended
			event = RelationUnitChangeEvent{
				Tag: w.relationTag,
				RemoteRelationChangeEvent: params.RemoteRelationChangeEvent{
					RelationToken:    w.remoteRelationToken,
					ApplicationToken: w.applicationToken,
					Life:             change.Life,
					Suspended:        &suspended,
					SuspendedReason:  change.SuspendedReason,
				},
			}
			changes = w.changes

		case changes <- event:
			changes = nil
		}
	}
}
