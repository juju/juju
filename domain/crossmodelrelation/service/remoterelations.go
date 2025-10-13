// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// GetRelationToken returns the token associated with the provided relation Key.
// Not implemented yet in the domain service.
func (w *WatchableService) GetRelationToken(ctx context.Context, relationKey string) (string, error) {
	return "", errors.Errorf("crossmodelrelation.GetToken").Add(coreerrors.NotImplemented)
}

// RemoteApplications returns the current state for the named remote applications.
// Not implemented yet in the domain service.
func (w *WatchableService) RemoteApplications(ctx context.Context, applications []string) ([]params.RemoteApplicationResult, error) {
	return nil, errors.Errorf("crossmodelrelation.RemoteApplications").Add(coreerrors.NotImplemented)
}

// WatchRemoteRelations returns a disabled watcher for remote relations for now.
// Not implemented yet in the domain service.
func (w *WatchableService) WatchRemoteRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	return nil, errors.Errorf("crossmodelrelation.WatchRemoteRelations").Add(coreerrors.NotImplemented)
}
