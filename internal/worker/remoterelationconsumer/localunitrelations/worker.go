// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localunitrelations

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/domain/relation"
)

// Service defines the interface required to watch for local relation changes.
type Service interface {
	// WatchRelationUnits returns a watcher for changes to the units
	// in the given relation in the local model.
	WatchRelationUnits(context.Context, coreapplication.UUID) (watcher.NotifyWatcher, error)

	// GetRelationUnits returns the current state of the relation units.
	GetRelationUnits(context.Context, coreapplication.UUID) (relation.RelationUnitChange, error)
}

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	worker.Reporter
}

// Config contains the configuration parameters for a remote relation units
// worker.
type Config struct {
	Service         Service
	ApplicationUUID coreapplication.UUID
	RelationUUID    corerelation.UUID

	Changes chan<- relation.RelationUnitChange

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Service == nil {
		return errors.NotValidf("service cannot be nil")
	}
	if c.ApplicationUUID.IsEmpty() {
		return errors.NotValidf("application UUID cannot be empty")
	}
	if c.RelationUUID.IsEmpty() {
		return errors.NotValidf("relation UUID cannot be empty")
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

	applicationUUID coreapplication.UUID
	relationUUID    corerelation.UUID
	changes         chan<- relation.RelationUnitChange

	clock  clock.Clock
	logger logger.Logger
}

// NewWorker creates a new worker that watches for local relation unit
// changes and sends them to the provided changes channel.
func NewWorker(cfg Config) (ReportableWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &localWorker{
		service:      cfg.Service,
		relationUUID: cfg.RelationUUID,
		changes:      cfg.Changes,
		clock:        cfg.Clock,
		logger:       cfg.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "local-relation-units",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", cfg.RelationUUID)
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

	watcher, err := w.service.WatchRelationUnits(ctx, w.applicationUUID)
	if err != nil {
		return errors.Annotatef(err, "watching local side of relation %v", w.relationUUID)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding watcher to catacomb for %v", w.relationUUID)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case _, ok := <-watcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("relation units watcher closed")
				}
			}

			w.logger.Debugf(ctx, "local relation units changed for %v", w.relationUUID)

			unitRelationInfo, err := w.service.GetRelationUnits(ctx, w.applicationUUID)
			if err != nil {
				return errors.Annotatef(
					err, "fetching local side of relation %v", w.relationUUID)
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

func isEmpty(change relation.RelationUnitChange) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}
