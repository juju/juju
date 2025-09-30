// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery"
	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/catacomb"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// RelationChange encapsulates a remote relation event.
type RelationChange struct {
	Tag                     names.RelationTag
	RelationToken           string
	ApplicationOrOfferToken string

	Life            life.Value
	Suspended       bool
	SuspendedReason string
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
	Client              RemoteModelRelationsClient
	RelationTag         names.RelationTag
	ApplicationToken    string
	LocalRelationToken  string
	RemoteRelationToken string
	Macaroon            *macaroon.Macaroon
	Changes             chan<- RelationChange
	Clock               clock.Clock
	Logger              logger.Logger
}

// Validate ensures the configuration is valid.
func (c Config) Validate() error {
	if c.Client == nil {
		return errors.NotValidf("remote model relations client cannot be nil")
	}
	if c.ApplicationToken == "" {
		return errors.NotValidf("application token cannot be empty")
	}
	if c.LocalRelationToken == "" {
		return errors.NotValidf("local relation token cannot be empty")
	}
	if c.RemoteRelationToken == "" {
		return errors.NotValidf("remote relation token cannot be empty")
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

	relationTag                             names.RelationTag
	localRelationToken, remoteRelationToken string
	applicationToken                        string
	changes                                 chan<- RelationChange

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
		client:              cfg.Client,
		macaroon:            cfg.Macaroon,
		relationTag:         cfg.RelationTag,
		localRelationToken:  cfg.LocalRelationToken,
		remoteRelationToken: cfg.RemoteRelationToken,
		applicationToken:    cfg.ApplicationToken,
		changes:             cfg.Changes,
		clock:               cfg.Clock,
		logger:              cfg.Logger,
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
		Token:         w.localRelationToken,
		Macaroons:     macaroon.Slice{w.macaroon},
		BakeryVersion: bakery.LatestVersion,
	})
	if err != nil {
		return errors.Annotatef(err, "watching remote side of relation for %q", w.relationTag)
	}

	if err := w.catacomb.Add(watcher); err != nil {
		return errors.Annotatef(err, "adding remote relation status watcher for %q", w.relationTag)
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
			if len(changes) == 0 {
				w.logger.Warningf(ctx, "relation status watcher event with no changes")
				continue
			}

			// We only care about the most recent change.
			change := changes[len(changes)-1]
			w.logger.Debugf(ctx, "relation status changed for %v: %v", w.relationTag, change)

			event := RelationChange{
				Tag:                     w.relationTag,
				RelationToken:           w.remoteRelationToken,
				ApplicationOrOfferToken: w.applicationToken,
				Life:                    change.Life,
				Suspended:               change.Suspended,
				SuspendedReason:         change.SuspendedReason,
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
