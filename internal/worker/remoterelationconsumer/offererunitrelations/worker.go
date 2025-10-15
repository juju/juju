// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package offererunitrelations

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

	// DeprecatedDepartedUnits represents the units that have departed in this
	// relation.
	// Deprecated: this will be removed in future releases in favour of using
	// AvailableUnits. We can then determine departed units by comparing
	// the previous set of available units with the current set.
	DeprecatedDepartedUnits []int

	// ApplicationSettings represent the updated application-level settings in
	// this relation.
	ApplicationSettings map[string]string

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
	Settings map[string]string
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

	// reportRequests is used to request a report of the current state.
	// This is to allow the worker to be reported on by the engine, without
	// needing to add locking around the state.
	reportRequests chan RelationUnitChange
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

		reportRequests: make(chan RelationUnitChange),
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

	var event RelationUnitChange
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

			appSettings := make(map[string]string)
			for k, v := range change.ApplicationSettings {
				switch val := v.(type) {
				case string:
					appSettings[k] = val
				default:
					w.logger.Errorf(ctx, "unexpected application setting value type %T for key %q", v, k)
					continue
				}
			}

			var unitSettings []UnitChange
			for _, c := range change.ChangedUnits {
				settings := make(map[string]string)
				for k, v := range c.Settings {
					switch val := v.(type) {
					case string:
						settings[k] = val
					default:
						w.logger.Errorf(ctx, "unexpected unit setting value type %T for key %q", v, k)
						continue
					}
				}
				unitSettings = append(unitSettings, UnitChange{
					UnitID:   c.UnitId,
					Settings: settings,
				})
			}

			event = RelationUnitChange{
				ConsumerRelationUUID:    w.consumerRelationUUID,
				OffererApplicationUUID:  w.offererApplicationUUID,
				ChangedUnits:            unitSettings,
				DeprecatedDepartedUnits: change.DepartedUnits,
				ApplicationSettings:     appSettings,
				Life:                    change.Life,
				Suspended:               unptr(change.Suspended, false),
				SuspendedReason:         change.SuspendedReason,
			}

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- event:
			}

		case w.reportRequests <- event:
		}
	}
}

// Report provides information for the engine report.
func (w *remoteWorker) Report() map[string]any {
	result := make(map[string]any)
	result["consumer-relation-uuid"] = w.consumerRelationUUID.String()
	result["offerer-application-uuid"] = w.offererApplicationUUID.String()

	select {
	case <-time.After(time.Second):
		result["error"] = "timed out waiting for report"

	case <-w.catacomb.Dying():
		result["error"] = "worker is dying"

	case event := <-w.reportRequests:
		result["changed-units"] = transform.Slice(event.ChangedUnits, func(c UnitChange) map[string]any {
			return map[string]any{
				"unit-id":  c.UnitID,
				"settings": c.Settings,
			}
		})
		result["settings"] = event.ApplicationSettings
		result["life"] = event.Life
		result["suspended"] = event.Suspended
		result["suspended-reason"] = event.SuspendedReason
		result["departed-units"] = event.DeprecatedDepartedUnits
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
