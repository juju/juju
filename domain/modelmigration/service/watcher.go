// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/core/watcher/eventsource"
)

// WatchImportClaims watches every target-side import claim. The collection
// contains model UUIDs initially and thereafter emits model UUIDs whose claim
// was inserted, updated, or deleted. Consumers must re-query claim state,
// since events can be coalesced.
func (s *WatchableService) WatchImportClaims(ctx context.Context) (watcher.StringsWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	namespace, initialQuery := s.controllerState.InitialWatchImportClaimsStatement()
	return s.watcherFactory.NewNamespaceWatcher(
		ctx,
		eventsource.InitialNamespaceChanges(initialQuery),
		"migration import claims watcher",
		eventsource.NamespaceFilter(namespace, changestream.All),
	)
}

// WatchModelDatabaseDeletion returns a notify watcher that fires when the staged
// model-database deletion for the given model changes, including when the
// undertaker's model-database deleter removes it after dropping the database.
// The abort finalization wait uses this to react to the drop completing instead
// of polling.
func (s *WatchableService) WatchModelDatabaseDeletion(
	ctx context.Context, modelUUID coremodel.UUID,
) (watcher.NotifyWatcher, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return s.watcherFactory.NewNotifyWatcher(
		ctx,
		"model database deletion watcher",
		eventsource.PredicateFilter(
			s.controllerState.NamespaceForWatchModelDatabaseDeletion(),
			changestream.All,
			eventsource.EqualsPredicate(modelUUID.String()),
		),
	)
}
