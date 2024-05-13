// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/internal/secrets/provider"
	"github.com/juju/juju/rpc/params"
)

// SecretBackendsManagerAPI is the implementation for the SecretsManager facade.
type SecretBackendsManagerAPI struct {
	watcherRegistry facade.WatcherRegistry

	backendService BackendService
	clock          clock.Clock
}

// WatchSecretBackendsRotateChanges sets up a watcher to notify of changes to secret backend rotations.
func (s *SecretBackendsManagerAPI) WatchSecretBackendsRotateChanges(ctx context.Context) (params.SecretBackendRotateWatchResult, error) {
	result := params.SecretBackendRotateWatchResult{}
	w, err := s.backendService.WatchSecretBackendRotationChanges()
	if err != nil {
		return result, errors.Trace(err)
	}

	id, backendChanges, err := internal.EnsureRegisterWatcher(ctx, s.watcherRegistry, w)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	changes := make([]params.SecretBackendRotateChange, len(backendChanges))
	for i, c := range backendChanges {
		changes[i] = params.SecretBackendRotateChange{
			ID:              c.ID,
			Name:            c.Name,
			NextTriggerTime: c.NextTriggerTime,
		}
	}

	result.WatcherId = id
	result.Changes = changes
	return result, nil
}

// For testing.
var (
	GetProvider = provider.Provider
)

// RotateBackendTokens rotates the tokens for the specified backends.
func (s *SecretBackendsManagerAPI) RotateBackendTokens(ctx context.Context, args params.RotateSecretBackendArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.BackendIDs)),
	}
	for i, backendID := range args.BackendIDs {
		err := s.backendService.RotateBackendToken(ctx, backendID)
		results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
	}
	return results, nil
}
