// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteunitrelations

import (
	"context"

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

	// Tag is the relation tag that this change relates to.
	Tag names.Tag
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
	Client         RemoteModelRelationsClient
	RelationTag    names.RelationTag
	RelationToken  string
	RemoteAppToken string
	Macaroon       *macaroon.Macaroon
	Changes        chan<- RelationUnitChange
	Clock          clock.Clock
	Logger         logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Client == nil {
		return errors.NotValidf("remote model relations client cannot be nil")
	}
	if c.RelationToken == "" {
		return errors.NotValidf("relation token cannot be empty")
	}
	if c.RemoteAppToken == "" {
		return errors.NotValidf("remote application token cannot be empty")
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

	relationTag    names.RelationTag
	relationToken  string
	remoteAppToken string

	changes chan<- RelationUnitChange

	clock  clock.Clock
	logger logger.Logger
}

// NewWorker creates a new worker that watches for remote relation unit
// changes and sends them to the provided changes channel.
func NewWorker(cfg Config) (ReportableWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &remoteWorker{
		client: cfg.Client,

		relationTag:    cfg.RelationTag,
		macaroon:       cfg.Macaroon,
		relationToken:  cfg.RelationToken,
		remoteAppToken: cfg.RemoteAppToken,

		changes: cfg.Changes,
		clock:   cfg.Clock,
		logger:  cfg.Logger,
	}
	if err := catacomb.Invoke(catacomb.Plan{
		Name: "relation-units",
		Site: &w.catacomb,
		Work: w.loop,
	}); err != nil {
		return nil, errors.Annotatef(err, "starting relation units worker for %v", cfg.RelationTag)
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
		ctx, w.relationToken, w.remoteAppToken, macaroon.Slice{w.macaroon},
	)
	if err != nil {
		return errors.Annotatef(err, "watching remote relation %v", w.relationTag.Id())
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes, ok := <-watcher.Changes():
			if !ok {
				// We are dying.
				return w.catacomb.ErrDying()
			}
			if isEmpty(changes) {
				continue
			}

			w.logger.Debugf(ctx, "remote relation units changed for %v: %v", w.relationTag, changes)

			// Send in lockstep so we don't drop events.
			select {
			case <-w.catacomb.Dying():
				return w.catacomb.ErrDying()
			case w.changes <- RelationUnitChange{}:
			}
		}
	}
}

// Report provides information for the engine report.
func (w *remoteWorker) Report() map[string]any {
	result := make(map[string]any)

	return result
}

func isEmpty(change params.RemoteRelationChangeEvent) bool {
	return len(change.ChangedUnits)+len(change.DepartedUnits) == 0 && change.ApplicationSettings == nil
}
