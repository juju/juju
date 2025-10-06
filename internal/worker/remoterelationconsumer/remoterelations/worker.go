// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	corerelation "github.com/juju/juju/core/relation"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// RelationChange encapsulates a remote relation event.
type RelationChange struct {
	ConsumerRelationUUID   corerelation.UUID
	OffererApplicationUUID coreapplication.UUID
	Life                   life.Value
	Suspended              bool
	SuspendedReason        string
}

// RemoteModelRelationsClient watches for changes to relations in a remote
// model.
type RemoteModelRelationsClient interface {
	// WatchRelationSuspendedStatus starts a RelationStatusWatcher for watching the
	// relations of each specified application in the remote model.
	WatchRelationSuspendedStatus(ctx context.Context, arg params.RemoteEntityArg) (watcher.RelationStatusWatcher, error)
}

// ReportableWorker is an interface that allows a worker to be reported
// on by the engine.
type ReportableWorker interface {
	worker.Worker
	worker.Reporter
}

// Config contains the configuration parameters for a remote relation worker.
type Config struct {
	Client                 RemoteModelRelationsClient
	ConsumerRelationUUID   corerelation.UUID
	OffererApplicationUUID coreapplication.UUID
	Macaroon               *macaroon.Macaroon
	Changes                chan<- RelationChange
	Clock                  clock.Clock
	Logger                 logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Client == nil {
		return errors.NotValidf("remote model relations client cannot be nil")
	}
	if c.ConsumerRelationUUID == "" {
		return errors.NotValidf("consumer relation token cannot be empty")
	}
	if c.OffererApplicationUUID == "" {
		return errors.NotValidf("offerer application UUID cannot be empty")
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

// remoteRelationsWorker listens for changes to the
// life and status of a relation in the offering model.
type remoteRelationsWorker struct {
	catacomb catacomb.Catacomb

	client   RemoteModelRelationsClient
	macaroon *macaroon.Macaroon

	consumerRelationUUID   corerelation.UUID
	offererApplicationUUID coreapplication.UUID
	changes                chan<- RelationChange

	clock  clock.Clock
	logger logger.Logger
}

// Worker creates a new worker that watches for changes
// to the life and status of a relation in a remote model.
func NewWorker(cfg Config) (ReportableWorker, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &remoteRelationsWorker{
		client:                 cfg.Client,
		macaroon:               cfg.Macaroon,
		consumerRelationUUID:   cfg.ConsumerRelationUUID,
		offererApplicationUUID: cfg.OffererApplicationUUID,
		changes:                cfg.Changes,
		clock:                  cfg.Clock,
		logger:                 cfg.Logger,
	}
	err := catacomb.Invoke(catacomb.Plan{
		Name: "remote-relations",
		Site: &w.catacomb,
		Work: w.loop,
	})
	return w, err
}

// Kill is defined on worker.Worker
func (w *remoteRelationsWorker) Kill() {
	w.catacomb.Kill(nil)
}

// Wait is defined on worker.Worker
func (w *remoteRelationsWorker) Wait() error {
	return w.catacomb.Wait()
}

func (w *remoteRelationsWorker) loop() error {
	ctx := w.catacomb.Context(context.Background())

	// Totally new so start the lifecycle watcher.
	watcher, err := w.client.WatchRelationSuspendedStatus(ctx, params.RemoteEntityArg{
		Token:         w.consumerRelationUUID.String(),
		Macaroons:     macaroon.Slice{w.macaroon},
		BakeryVersion: bakery.LatestVersion,
	})
	if err != nil {
		return errors.Annotatef(err, "watching offerer side of relation for %q", w.consumerRelationUUID)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding offerer relation status watcher for %q", w.consumerRelationUUID)
	}

	for {
		select {
		case <-w.catacomb.Dying():
			return w.catacomb.ErrDying()

		case changes, ok := <-watcher.Changes():
			if !ok {
				select {
				case <-w.catacomb.Dying():
					return w.catacomb.ErrDying()
				default:
					return errors.NotValidf("relation status watcher closed unexpectedly for %q", w.consumerRelationUUID)
				}
			}
			if len(changes) == 0 {
				w.logger.Warningf(ctx, "relation status watcher event with no changes")
				continue
			}

			// We only care about the most recent change.
			change := changes[len(changes)-1]
			w.logger.Debugf(ctx, "relation status changed for %v: %v", w.consumerRelationUUID, change)

			event := RelationChange{
				ConsumerRelationUUID:   w.consumerRelationUUID,
				OffererApplicationUUID: w.offererApplicationUUID,
				Life:                   change.Life,
				Suspended:              change.Suspended,
				SuspendedReason:        change.SuspendedReason,
			}

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

	return result
}
