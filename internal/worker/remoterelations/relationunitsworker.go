// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"
	"sync"
	"time"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
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

// Mode indicates whether the relation units worker is for a local or remote
// model.
type Mode string

const (
	// ModeLocal indicates that the worker is for a local model.
	ModeLocal Mode = "local"
	// ModeRemote indicates that the worker is for a remote model.
	ModeRemote Mode = "remote"
)

// relationUnitsWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type relationUnitsWorker struct {
	catacomb catacomb.Catacomb

	mode Mode

	// mostRecentChange is stored here for the engine report.
	mu               sync.Mutex
	mostRecentChange RelationUnitChangeEvent
	changeSince      time.Time

	relationTag names.RelationTag
	watcher     watcher.RemoteRelationWatcher
	changes     chan<- RelationUnitChangeEvent

	mapEvent func(RelationUnitChangeEvent) RelationUnitChangeEvent

	clock  clock.Clock
	logger logger.Logger
}

func newLocalRelationUnitsWorker(
	ctx context.Context,
	facade RemoteRelationsFacade,
	relationTag names.RelationTag,
	mac *macaroon.Macaroon,
	changes chan<- RelationUnitChangeEvent,
	clock clock.Clock,
	logger logger.Logger,
) (ReportableWorker, error) {
	// Start a watcher to track changes to the units in the relation in the
	// local model.
	watcher, err := facade.WatchLocalRelationChanges(ctx, relationTag.Id())
	if err != nil {
		return nil, errors.Annotatef(err, "watching local side of relation %v", relationTag.Id())
	}

	w := &relationUnitsWorker{
		mode:        ModeLocal,
		relationTag: relationTag,
		watcher:     watcher,
		changes:     changes,
		mapEvent: func(event RelationUnitChangeEvent) RelationUnitChangeEvent {
			event.Macaroons = macaroon.Slice{mac}
			event.BakeryVersion = bakery.LatestVersion
			return event
		},
		clock:  clock,
		logger: logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{watcher},
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", relationTag)
	}
	return w, nil
}

func newRemoteRelationUnitsWorker(
	ctx context.Context,
	facade RemoteModelRelationsFacade,
	relationTag names.RelationTag,
	mac *macaroon.Macaroon,
	relationToken, remoteAppToken string,
	applicationName string,
	changes chan<- RelationUnitChangeEvent,
	clock clock.Clock,
	logger logger.Logger,
) (ReportableWorker, error) {
	// Start a watcher to track changes to the units in the relation in the
	// remote model.
	watcher, err := facade.WatchRelationChanges(
		ctx, relationToken, remoteAppToken, macaroon.Slice{mac},
	)
	if err != nil {
		return nil, errors.Annotatef(
			err, "watching remote side of application %v and relation %v",
			applicationName, relationTag.Id())
	}

	w := &relationUnitsWorker{
		relationTag: relationTag,
		watcher:     watcher,
		changes:     changes,
		clock:       clock,
		logger:      logger,
		mapEvent:    identityMap,
		mode:        ModeRemote,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units",
		Site: &w.catacomb,
		Work: w.loop,
		Init: []worker.Worker{watcher},
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", relationTag)
	}
	return w, nil
}

// Kill stops the worker. If the worker is already dying, it does nothing.
func (w *relationUnitsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. If the worker has been killed, it will
// return the error.
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
	ctx := w.catacomb.Context(context.Background())

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()
		case change, ok := <-w.watcher.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			if isEmpty(change) {
				continue
			}

			w.logger.Debugf(ctx, "%v relation units changed for %v: %#v", w.mode, w.relationTag, &change)

			event := w.mapEvent(RelationUnitChangeEvent{
				Tag:                       w.relationTag,
				RemoteRelationChangeEvent: change,
			})

			w.mu.Lock()
			w.mostRecentChange = event
			w.changeSince = w.clock.Now()
			w.mu.Unlock()

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- event:
			}
		}
	}
}

// Report provides information for the engine report.
func (w *relationUnitsWorker) Report() map[string]any {
	result := make(map[string]any)

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

func isEmpty(change params.RemoteRelationChangeEvent) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}

func identityMap(event RelationUnitChangeEvent) RelationUnitChangeEvent {
	return event
}
