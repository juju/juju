// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteunitrelations

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api/watcher"
	coreapplication "github.com/juju/juju/core/application"
	corelife "github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/rpc/params"
)

// RelationUnitChange encapsulates a remote relation event,
// adding the tag of the relation which changed.
type RelationUnitChange struct {
	// ConsumerRelationUUID is the UUID of the relation in the local model.
	ConsumerRelationUUID corerelation.UUID

	// OffererApplicationUUID is the UUID of the application in the remote
	// model that is offering the relation.
	OffererApplicationUUID coreapplication.UUID

	// ChangedUnits represents the changed units in this relation.
	ChangedUnits []UnitChange

	// LegacyDepartedUnits represents the units that have departed in this
	// relation.
	LegacyDepartedUnits []int

	// ApplicationSettings represent the updated application-level settings in
	// this relation.
	ApplicationSettings map[string]any

	// UnitCount is the total number of units in the relation.
	UnitCount int

	// Life is the current life state of the relation.
	Life corelife.Value

	// Suspended indicates whether the relation is currently suspended.
	Suspended bool

	// SuspendedReason provides additional context if the relation is suspended.
	SuspendedReason string
}

// UnitChange represents a change to a single unit in a relation.
type UnitChange struct {
	// UnitId uniquely identifies the remote unit.
	UnitID int

	// Settings is the current settings for the relation unit.
	Settings map[string]any
}

// RemoteModelRelationsClient watches for changes to relations in a remote
// model.
type RemoteModelRelationsClient interface {
	// WatchRelationChanges returns a watcher that notifies of changes
	// to the units in the remote model for the relation with the
	// given remote token. We need to pass the application token for
	// the case where we're talking to a v1 API and the client needs
	// to convert RelationUnitsChanges into RemoteRelationChangeEvents
	// as they come in.
	WatchRelationChanges(ctx context.Context, relationToken, applicationToken string, macs macaroon.Slice) (watcher.RemoteRelationWatcher, error)
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
	Client                 RemoteModelRelationsClient
	ConsumerRelationUUID   corerelation.UUID
	OffererApplicationUUID coreapplication.UUID
	Macaroon               *macaroon.Macaroon
	Changes                chan<- RelationUnitChange
	Clock                  clock.Clock
	Logger                 logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Client == nil {
		return errors.NotValidf("remote model relations client cannot be nil")
	}
	if c.ConsumerRelationUUID == "" {
		return errors.NotValidf("consumer relation uuid cannot be empty")
	}
	if c.OffererApplicationUUID == "" {
		return errors.NotValidf("offerer application token cannot be empty")
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

// remoteWorker uses instances of watcher.RelationUnitsWatcher to
// listen to changes to relation settings in a model, local or remote.
// Local changes are exported to the remote model.
// Remote changes are consumed by the local model.
type remoteWorker struct {
	catacomb catacomb.Catacomb

	client   RemoteModelRelationsClient
	macaroon *macaroon.Macaroon

	consumerRelationUUID   corerelation.UUID
	offererApplicationUUID coreapplication.UUID

	changes chan<- RelationUnitChange

	clock  clock.Clock
	logger logger.Logger

	requests chan chan map[string]any
}

// NewWorker creates a new worker that watches for remote relation unit
// changes and sends them to the provided changes channel.
func NewWorker(cfg Config) (ReportableWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	w := &remoteWorker{
		client: cfg.Client,

		consumerRelationUUID:   cfg.ConsumerRelationUUID,
		offererApplicationUUID: cfg.OffererApplicationUUID,
		macaroon:               cfg.Macaroon,

		changes: cfg.Changes,
		clock:   cfg.Clock,
		logger:  cfg.Logger,

		requests: make(chan chan map[string]any),
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", cfg.ConsumerRelationUUID)
	}
	return w, nil
}

// Kill stops the worker. If the worker is already dying, it does nothing.
func (w *remoteWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait waits for the worker to finish. If the worker has been killed, it will
// return the error.
func (w *remoteWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	// Start a watcher to track changes to the units in the relation in the
	// remote model.
	watcher, err := w.client.WatchRelationChanges(
		ctx, w.consumerRelationUUID.String(), w.offererApplicationUUID.String(), macaroon.Slice{w.macaroon},
	)
	if err != nil {
		return errors.Annotatef(err, "watching remote relation %v", w.consumerRelationUUID)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding remote relation units watcher for %v", w.consumerRelationUUID)
	}

	var event params.RemoteRelationChangeEvent
	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case change, ok := <-watcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.New("remote relation units watcher closed")
				}
			}
			if isEmpty(change) {
				continue
			}

			w.logger.Debugf(ctx, "remote relation units changed for %v: %v", w.consumerRelationUUID, change)

			event = change

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- RelationUnitChange{
				ConsumerRelationUUID:   w.consumerRelationUUID,
				OffererApplicationUUID: w.offererApplicationUUID,
				ChangedUnits: transform.Slice(change.ChangedUnits, func(c params.RemoteRelationUnitChange) UnitChange {
					return UnitChange{
						UnitID:   c.UnitId,
						Settings: c.Settings,
					}
				}),
				LegacyDepartedUnits: change.DepartedUnits,
				ApplicationSettings: change.ApplicationSettings,
				UnitCount:           change.UnitCount,
				Life:                change.Life,
				Suspended:           unptr(change.Suspended, false),
				SuspendedReason:     change.SuspendedReason,
			}:
			}

		case resp := <-w.requests:
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()

			case resp <- map[string]any{
				"application-uuid": w.offererApplicationUUID.String(),
				"relation-uuid":    w.consumerRelationUUID.String(),
				"unit-count":       event.UnitCount,
				"changed-units": transform.Slice(event.ChangedUnits, func(c params.RemoteRelationUnitChange) map[string]any {
					return map[string]any{
						"unit-id":  c.UnitId,
						"settings": c.Settings,
					}
				}),
				"settings":         event.ApplicationSettings,
				"life":             event.Life,
				"suspended":        unptr(event.Suspended, false),
				"suspended-reason": event.SuspendedReason,

				// This only exists for legacy reasons (3.x compatibility).
				// Although it's a good proxy for if the relation has changed.
				"legacy-departed-units": event.DepartedUnits,
			}:
			}
		}
	}
}

// Report provides information for the engine report.
func (w *remoteWorker) Report() map[string]any {
	result := make(map[string]any)

	ch := make(chan map[string]any, 1)
	select {
	case <-w.catacomb.Dying():
		return result
	case <-w.clock.After(time.Second):
		// Timeout trying to report.
		result["error"] = "timed out trying to report"
		return result
	case w.requests <- ch:
	}

	select {
	case <-w.catacomb.Dying():
		return result
	case <-w.clock.After(time.Second):
		// Timeout trying to report.
		result["error"] = "timed out waiting for report response"
		return result
	case resp := <-ch:
		result = resp
	}

	return result
}

func isEmpty(change params.RemoteRelationChangeEvent) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}

func unptr[T any](v *T, d T) T {
	if v == nil {
		return d
	}
	return *v
}
