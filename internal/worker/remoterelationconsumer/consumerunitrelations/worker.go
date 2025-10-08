// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package consumerunitrelations

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

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

// RelationUnitChange encapsulates a local relation event, adding the macaroon
// required to authenticate with the remote model.
type RelationUnitChange struct {
	relation.RelationUnitChange
	Macaroon *macaroon.Macaroon
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
	Service                 Service
	ConsumerApplicationUUID coreapplication.UUID
	ConsumerRelationUUID    corerelation.UUID

	Macaroon *macaroon.Macaroon

	Changes chan<- RelationUnitChange

	Clock  clock.Clock
	Logger logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Service == nil {
		return errors.NotValidf("service cannot be nil")
	}
	if c.ConsumerApplicationUUID.IsEmpty() {
		return errors.NotValidf("application UUID cannot be empty")
	}
	if c.ConsumerRelationUUID.IsEmpty() {
		return errors.NotValidf("relation UUID cannot be empty")
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

	consumerApplicationUUID coreapplication.UUID
	consumerRelationUUID    corerelation.UUID

	macaroon *macaroon.Macaroon

	changes chan<- RelationUnitChange

	clock  clock.Clock
	logger logger.Logger

	// reportRequests is used to request a report of the current state.
	// This is to allow the worker to be reported on by the engine, without
	// needing to add locking around the state.
	reportRequests chan relation.RelationUnitChange
}

// NewWorker creates a new worker that watches for local relation unit
// changes and sends them to the provided changes channel.
func NewWorker(cfg Config) (ReportableWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &localWorker{
		service: cfg.Service,

		consumerRelationUUID:    cfg.ConsumerRelationUUID,
		consumerApplicationUUID: cfg.ConsumerApplicationUUID,

		macaroon: cfg.Macaroon,

		changes: cfg.Changes,
		clock:   cfg.Clock,
		logger:  cfg.Logger,

		reportRequests: make(chan relation.RelationUnitChange),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "local-relation-units",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", cfg.ConsumerRelationUUID)
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

	watcher, err := w.service.WatchRelationUnits(ctx, w.consumerApplicationUUID)
	if err != nil {
		return errors.Annotatef(err, "watching local side of relation %v", w.consumerRelationUUID)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding watcher to catacomb for %v", w.consumerRelationUUID)
	}

	var event relation.RelationUnitChange
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

			w.logger.Debugf(ctx, "local relation units changed for %v", w.consumerRelationUUID)

			unitRelationInfo, err := w.service.GetRelationUnits(ctx, w.consumerApplicationUUID)
			if err != nil {
				return errors.Annotatef(
					err, "fetching local side of relation %v", w.consumerRelationUUID)
			}

			if isEmpty(unitRelationInfo) {
				continue
			}

			event = unitRelationInfo

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- RelationUnitChange{
				RelationUnitChange: event,
				Macaroon:           w.macaroon,
			}:
			}

		case w.reportRequests <- event:
		}
	}
}

// Report provides information for the engine report.
func (w *localWorker) Report() map[string]any {
	result := make(map[string]any)
	result["consumer-relation-uuid"] = w.consumerRelationUUID.String()
	result["consumer-application-uuid"] = w.consumerApplicationUUID.String()

	select {
	case <-time.After(time.Second):
		result["error"] = "timed out waiting for report"

	case <-w.catacomb.Dying():
		result["error"] = "worker is dying"

	case event := <-w.reportRequests:
		result["changed-units"] = event.ChangedUnits
		result["all-units"] = event.AllUnits
		result["in-scope-units"] = event.InScopeUnits
		result["settings"] = event.ApplicationSettings
	}

	return result
}

func isEmpty(change relation.RelationUnitChange) bool {
	return len(change.ChangedUnits)+len(change.InScopeUnits) == 0
}
