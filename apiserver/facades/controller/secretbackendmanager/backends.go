// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackendmanager

import (
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	coresecrets "github.com/juju/juju/core/secrets"
	corewatcher "github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// SecretBackendsManagerAPI is the implementation for the SecretsManager facade.
type SecretBackendsManagerAPI struct {
	watcherRegistry facade.WatcherRegistry

	controllerUUID string
	modelUUID      string
	modelName      string

	backendRotate BackendRotate
	backendState  BackendState
	clock         clock.Clock
	logger        loggo.Logger
}

// WatchSecretBackendsRotateChanges sets up a watcher to notify of changes to secret backend rotations.
func (s *SecretBackendsManagerAPI) WatchSecretBackendsRotateChanges() (params.SecretBackendRotateWatchResult, error) {
	result := params.SecretBackendRotateWatchResult{}
	w, err := s.backendRotate.WatchSecretBackendRotationChanges()
	if err != nil {
		return result, errors.Trace(err)
	}

	id, backendChanges, err := internal.EnsureRegisterWatcher[[]corewatcher.SecretBackendRotateChange](s.watcherRegistry, w)
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
func (s *SecretBackendsManagerAPI) RotateBackendTokens(args params.RotateSecretBackendArgs) (params.ErrorResults, error) {
	results := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.BackendIDs)),
	}
	for i, backendID := range args.BackendIDs {
		backendInfo, err := s.backendState.GetSecretBackendByID(backendID)
		if err != nil {
			results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
			continue
		}
		p, err := GetProvider(backendInfo.BackendType)
		if err != nil {
			results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
			continue
		}
		if !provider.HasAuthRefresh(p) {
			continue
		}

		if backendInfo.TokenRotateInterval == nil || *backendInfo.TokenRotateInterval == 0 {
			s.logger.Warningf("not rotating token for secret backend %q", backendInfo.Name)
			continue
		}

		s.logger.Debugf("refresh token for backend %v", backendInfo.Name)
		cfg := &provider.ModelBackendConfig{
			ControllerUUID: s.controllerUUID,
			ModelUUID:      s.modelUUID,
			ModelName:      s.modelName,
			BackendConfig: provider.BackendConfig{
				BackendType: backendInfo.BackendType,
				Config:      backendInfo.Config,
			},
		}

		var nextRotateTime time.Time
		auth, err := p.(provider.SupportAuthRefresh).RefreshAuth(cfg, *backendInfo.TokenRotateInterval)
		if err != nil {
			s.logger.Errorf("refreshing auth token for %q: %v", backendInfo.Name, err)
			results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
			// If there's a permission error, we can't recover from that.
			if errors.Is(err, secrets.PermissionDenied) {
				continue
			}
		} else {
			err = s.backendState.UpdateSecretBackend(state.UpdateSecretBackendParams{
				ID:     backendID,
				Config: auth.Config,
			})
			if err != nil {
				results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
			} else {
				next, _ := coresecrets.NextBackendRotateTime(s.clock.Now(), *backendInfo.TokenRotateInterval)
				nextRotateTime = *next
			}
		}

		if nextRotateTime.IsZero() {
			nextRotateTime = s.clock.Now().Add(2 * time.Minute)
		}
		s.logger.Debugf("updating token rotation for %q, next: %s", backendInfo.Name, nextRotateTime)
		err = s.backendState.SecretBackendRotated(backendID, nextRotateTime)
		if err != nil {
			results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
		}
	}
	return results, nil
}
