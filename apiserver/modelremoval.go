// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"context"

	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/watcher"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// ModelRemovalWatchService creates watchers that emit when a model changes.
// It is satisfied by the controller-scoped *modelservice.WatchableService.
type ModelRemovalWatchService interface {
	// WatchModel returns a watcher that emits an event if the model changes.
	WatchModel(ctx context.Context, modelUUID coremodel.UUID) (watcher.NotifyWatcher, error)
}

// servedConnection is the subset of *rpc.Conn needed by the model removal
// watch. It exists so the watch can be unit tested without a full RPC
// connection.
type servedConnection interface {
	// Dead returns a channel that is closed when the connection dies.
	Dead() <-chan struct{}
	// Close closes the connection. Closing an already dead or closed
	// connection is not an error.
	Close() error
}

// watchServedModelRemoval arranges for conn to be closed when the model it
// serves is removed from this controller. Model removal happens when a model
// is destroyed or when a migration's REAP phase deletes it after migrating
// away; agents and clients must then reconnect, dialling the addresses the
// migration wrote into their configuration.
//
// The watch is best-effort: if it cannot be established the connection is
// served without it, as it always has been.
func (srv *Server) watchServedModelRemoval(
	ctx context.Context,
	conn servedConnection,
	modelService ModelService,
	watchService ModelRemovalWatchService,
	modelUUID coremodel.UUID,
) {
	// TODO: WatchModel fires on any change to the model; this path could
	// be optimised to use a specific life trigger (e.g. WatchModelLife)
	// only, so unrelated model updates do not wake the watch.
	watch, err := watchService.WatchModel(ctx, modelUUID)
	if err != nil {
		logger.Warningf(ctx,
			"cannot watch model %q for removal; connection will not be closed on model removal: %v",
			modelUUID, err,
		)
		return
	}
	// The goroutine is deliberately not added to the server's catacomb:
	// ctx is the connection's context, which the catacomb already cancels
	// on shutdown, and the watch also stops when the connection dies.
	// Catacomb membership would instead make a watch failure fatal to the
	// whole apiserver.
	go srv.runModelRemovalWatch(ctx, conn, modelService, watch, modelUUID)
}

// runModelRemovalWatch closes conn once the model it serves no longer exists
// on this controller. It returns when the connection dies, the context is
// cancelled, or the watcher stops.
func (srv *Server) runModelRemovalWatch(
	ctx context.Context,
	conn servedConnection,
	modelService ModelService,
	watch watcher.NotifyWatcher,
	modelUUID coremodel.UUID,
) {
	defer func() {
		watch.Kill()
		_ = watch.Wait()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-conn.Dead():
			// The webserver serving this connection closes it once it
			// dies (see Server.apiHandler), so this branch only stops
			// the watch; there is no connection to close here.
			return
		case _, ok := <-watch.Changes():
			if !ok {
				logger.Debugf(ctx,
					"removal watcher for model %q stopped; connection will not be closed on model removal",
					modelUUID,
				)
				return
			}
			connInfo, err := modelIsConnectable(ctx, modelService, modelUUID)
			if errors.Is(err, modelerrors.NotFound) || (err == nil && !connInfo.connectable) {
				logger.Infof(ctx, "model %q has been removed, closing connection", modelUUID)
				_ = conn.Close()
				return
			}
			// The model still exists, or the check failed transiently;
			// keep watching.
		}
	}
}
