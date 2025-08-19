// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// remoteRelationsWorker listens for changes to the
// life and status of a relation in the offering model.
type remoteRelationsWorker struct {
	catacomb catacomb.Catacomb

	mu sync.Mutex

	// mostRecentEvent is stored here for the engine report.
	mostRecentEvent RelationUnitChangeEvent
	changeSince     time.Time

	relationTag         names.RelationTag
	remoteRelationToken string
	applicationToken    string
	relationsWatcher    watcher.RelationStatusWatcher
	changes             chan<- RelationUnitChangeEvent

	clock  clock.Clock
	logger logger.Logger
}

func newRemoteRelationsWorker(
	relationTag names.RelationTag,
	applicationToken string,
	remoteRelationToken string,
	relationsWatcher watcher.RelationStatusWatcher,
	changes chan<- RelationUnitChangeEvent,
	clock clock.Clock,
	logger logger.Logger,
) (ReportableWorker, error) {
	w := &remoteRelationsWorker{
		relationsWatcher:    relationsWatcher,
		relationTag:         relationTag,
		remoteRelationToken: remoteRelationToken,
		applicationToken:    applicationToken,
		changes:             changes,
		clock:               clock,
		logger:              logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-relations",
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
		w.logger.Errorf(context.Background(), "error in remote relations worker for relation %v: %v", w.relationTag.Id(), err)
	}
	return err
}

func (w *remoteRelationsWorker) loop() error {
	ctx, cancel := w.scopeContext()
	defer cancel()

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
				w.logger.Warningf(ctx, "relation status watcher event with no changes")
				continue
			}

			// We only care about the most recent change.
			change := relChanges[len(relChanges)-1]
			w.logger.Debugf(ctx, "relation status changed for %v: %v", w.relationTag, change)
			suspended := change.Suspended

			w.mu.Lock()
			w.mostRecentEvent = RelationUnitChangeEvent{
				Tag: w.relationTag,
				RemoteRelationChangeEvent: params.RemoteRelationChangeEvent{
					RelationToken:           w.remoteRelationToken,
					ApplicationOrOfferToken: w.applicationToken,
					Life:                    change.Life,
					Suspended:               &suspended,
					SuspendedReason:         change.SuspendedReason,
				},
			}
			w.changeSince = w.clock.Now()
			event := w.mostRecentEvent
			w.mu.Unlock()

			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- event:
			}
		}
	}
}

// Report provides information for the engine report.
func (w *remoteRelationsWorker) Report() map[string]interface{} {
	result := make(map[string]interface{})
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.mostRecentEvent.Tag.Id() != "" {
		result["life"] = w.mostRecentEvent.Life
		result["suspended"] = w.mostRecentEvent.Suspended
		result["since"] = w.changeSince.Format(time.RFC1123Z)
	}

	return result
}

func (w *remoteRelationsWorker) scopeContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
