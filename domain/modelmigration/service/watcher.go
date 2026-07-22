// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/changestream"
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
