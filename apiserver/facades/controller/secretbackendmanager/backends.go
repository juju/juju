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
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.secretbackendmanager")

// SecretBackendsManagerAPI is the implementation for the SecretsManager facade.
type SecretBackendsManagerAPI struct {
	resources facade.Resources

	controllerUUID string
	modelUUID      string
	modelName      string

	backendRotate BackendRotate
	backendState  BackendState
	clock         clock.Clock
}

// WatchSecretBackendsRotateChanges sets up a watcher to notify of changes to secret backend rotations.
func (s *SecretBackendsManagerAPI) WatchSecretBackendsRotateChanges() (params.SecretBackendRotateWatchResult, error) {
	result := params.SecretBackendRotateWatchResult{}
	w, err := s.backendRotate.WatchSecretBackendRotationChanges()
	if err != nil {
		return result, errors.Trace(err)
	}
	if backendChanges, ok := <-w.Changes(); ok {
		changes := make([]params.SecretBackendRotateChange, len(backendChanges))
		for i, c := range backendChanges {
			changes[i] = params.SecretBackendRotateChange{
				ID:              c.ID,
				Name:            c.Name,
				NextTriggerTime: c.NextTriggerTime,
			}
		}
		result.WatcherId = s.resources.Register(w)
		result.Changes = changes
	} else {
		err = watcher.EnsureErr(w)
		result.Error = apiservererrors.ServerError(err)
	}
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
			logger.Warningf("not rotating token for secret backend %q", backendInfo.Name)
			continue
		}

		logger.Debugf("refresh token for backend %v", backendInfo.Name)
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
			logger.Errorf("refreshing auth token for %q: %v", backendInfo.Name, err)
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
		logger.Debugf("updating token rotation for %q, next: %s", backendInfo.Name, nextRotateTime)
		err = s.backendState.SecretBackendRotated(backendID, nextRotateTime)
		if err != nil {
			results.Results[i] = params.ErrorResult{Error: apiservererrors.ServerError(err)}
		}
	}
	return results, nil
}
