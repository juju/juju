// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localunitrelations

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
)

// RelationUnitChange encapsulates a remote relation event,
// adding the tag of the relation which changed.
type RelationUnitChange struct {
	// ChangedUnits represents the changed units in this relation.
	ChangedUnits []UnitChange

	// DepartedUnits represents the units that have departed in this relation.
	DepartedUnits []int

	// ApplicationSettings represent the updated application-level settings in
	// this relation.
	ApplicationSettings map[string]any
}

// UnitChange represents a change to a single unit in a relation.
type UnitChange struct {
	// UnitId uniquely identifies the remote unit.
	UnitID int

	// Settings is the current settings for the relation unit.
	Settings map[string]any
}

// Service defines the interface required to watch for local relation changes.
type Service interface {
	// WatchLocalRelationChanges returns a watcher for changes to the units
	// in the given relation in the local model.
	WatchLocalRelationChanges(ctx context.Context, relationID string) (watcher.NotifyWatcher, error)

	// GetUnitRelation returns the current state of the unit relation.
	GetUnitRelation(ctx context.Context, relationID string) (RelationUnitChange, error)
}

// Config contains the configuration parameters for a remote relation units
// worker.
type Config struct {
	Service     Service
	RelationTag names.RelationTag
	Macaroon    *macaroon.Macaroon

	Changes chan<- RelationUnitChange

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Service == nil {
		return errors.NotValidf("service cannot be nil")
	}
	if c.Macaroon == nil {
		return errors.NotValidf("macaroon cannot be nil")
	}
	if c.Changes == nil {
		return errors.NotValidf("changes channel cannot be nil")
	}
	if c.Clock == nil {
		return errors.NotValidf("clock cannot be nil")
	}
	if c.Logger == nil {
		return errors.NotValidf("logger cannot be nil")
	}
	return nil
}

// localWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
type localWorker struct {
	catacomb catacomb.Catacomb

	service Service

	relationTag names.RelationTag
	changes     chan<- RelationUnitChange

	clock  clock.Clock
	logger logger.Logger
}

// NewWorker creates a new worker that watches for local relation unit
// changes and sends them to the provided changes channel.
func NewWorker(cfg Config) (worker.Worker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &localWorker{
		service:     cfg.Service,
		relationTag: cfg.RelationTag,
		changes:     cfg.Changes,
		clock:       cfg.Clock,
		logger:      cfg.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "local-relation-units",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", cfg.RelationTag)
	}
	return w, nil
}

// Kill stops the worker. If the worker is already dying, it does nothing.
func (w *localWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. If the worker has been killed, it will
// return the error.
func (w *localWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *localWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	watcher, err := w.service.WatchLocalRelationChanges(ctx, w.relationTag.Id())
	if err != nil {
		return errors.Annotatef(err, "watching local side of relation %v", w.relationTag.Id())
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding watcher to catacomb for %v", w.relationTag)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-watcher.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}

			w.logger.Debugf(ctx, "local relation units changed for %v", w.relationTag)

			unitRelationInfo, err := w.service.GetUnitRelation(ctx, w.relationTag.Id())
			if err != nil {
				return errors.Annotatef(
					err, "fetching local side of relation %v", w.relationTag.Id())
			}

			if isEmpty(unitRelationInfo) {
				continue
			}

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- unitRelationInfo:
			}
		}
	}
}

// Report provides information for the engine report.
func (w *localWorker) Report() map[string]any {
	result := make(map[string]any)

	return result
}

func isEmpty(change RelationUnitChange) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}
