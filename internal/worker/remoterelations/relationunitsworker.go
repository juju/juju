// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/rpc/params"
)

// RelationUnitChangeEvent encapsulates a remote relation event,
// adding the tag of the relation which changed.
type RelationUnitChangeEvent struct {
	Tag names.RelationTag
	params.RemoteRelationChangeEvent
}

// relationUnitsWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type relationUnitsWorker struct {
	catacomb catacomb.Catacomb

	mu sync.Mutex

	// mostRecentChange is stored here for the engine report.
	mostRecentChange RelationUnitChangeEvent
	changeSince      time.Time

	relationTag names.RelationTag
	rrw         watcher.RemoteRelationWatcher
	changes     chan<- RelationUnitChangeEvent
	macaroon    *macaroon.Macaroon
	mode        string // mode is local or remote.

	logger logger.Logger
}

func newRelationUnitsWorker(
	relationTag names.RelationTag,
	macaroon *macaroon.Macaroon,
	rrw watcher.RemoteRelationWatcher,
	changes chan<- RelationUnitChangeEvent,
	logger logger.Logger,
	mode string,
) (*relationUnitsWorker, error) {
	w := &relationUnitsWorker{
		relationTag: relationTag,
		macaroon:    macaroon,
		rrw:         rrw,
		changes:     changes,
		logger:      logger,
		mode:        mode,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units",
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
	if errors.Is(err, errors.NotFound) || params.IsCodeNotFound(err) {
		err = nil
	}
	if err != nil {
		w.logger.Errorf(context.Background(), "error in relation units worker for %v: %v", w.relationTag.Id(), err)
	}
	return err
}

func (w *relationUnitsWorker) loop() error {
	ctx, cancel := w.scopeContext()
	defer cancel()

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.rrw.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			w.logger.Debugf(ctx, "%v relation units changed for %v: %#v", w.mode, w.relationTag, &change)
			if isEmpty(change) {
				continue
			}

			// Add macaroon in case this event is sent to a remote
			// facade.

			// TODO(babbageclunk): move this so it happens just before
			// the event is published to the remote facade.
			change.Macaroons = macaroon.Slice{w.macaroon}
			change.BakeryVersion = bakery.LatestVersion

			w.mu.Lock()
			w.mostRecentChange = RelationUnitChangeEvent{
				Tag:                       w.relationTag,
				RemoteRelationChangeEvent: change,
			}
			w.changeSince = time.Now()
			event := w.mostRecentChange
			w.mu.Unlock()
			// Send in lockstep so we don't drop events (otherwise
			// we'd need to merge them - not too hard in this
			// case but probably not needed).
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- event:
			}
		}
	}
}

func isEmpty(change params.RemoteRelationChangeEvent) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}

// Report provides information for the engine report.
func (w *relationUnitsWorker) Report() map[string]interface{} {
	result := make(map[string]interface{})
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.mostRecentChange.Tag.Id() != "" {
		var changed []int
		for _, c := range w.mostRecentChange.ChangedUnits {
			changed = append(changed, c.UnitId)
		}
		result["departed"] = w.mostRecentChange.DepartedUnits
		result["changed"] = changed
		result["since"] = w.changeSince.Format(time.RFC1123Z)
	}

	return result
}

func (w *relationUnitsWorker) scopeContext() (context.Context, context.CancelFunc) {
	return context.WithCancel(w.catacomb.Context(context.Background()))
}
