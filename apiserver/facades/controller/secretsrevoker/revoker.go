// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/rpc/params"
	secretsprovider "github.com/juju/juju/secrets/provider"
)

var logger = loggo.GetLogger("juju.apiserver.secretsrevoker")

// SecretsRevokerAPI is the implementation for the SecretsRevoker facade.
type SecretsRevokerAPI struct {
	state SecretsState

	resources facade.Resources

	backendConfigGetter commonsecrets.BackendAdminConfigGetter
	providerGetter      func(string) (secretsprovider.SecretBackendProvider, error)
}

func (api *SecretsRevokerAPI) WatchIssuedTokenExpiry() (params.StringsWatchResult, error) {
	result := params.StringsWatchResult{}
	watch := api.state.WatchSecretBackendIssuedTokenExpiry()
	if changes, ok := <-watch.Changes(); ok {
		result.StringsWatcherId = api.resources.Register(watch)
		result.Changes = changes
	} else {
		return result, errors.Errorf("cannot obtain token expiry times")
	}
	return result, nil
}

func (api *SecretsRevokerAPI) RevokeIssuedTokens(
	until time.Time,
) (params.ErrorResult, error) {
	result := params.ErrorResult{}

	err := api.revokeIssuedTokens(until)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}

	return result, nil
}

func (api *SecretsRevokerAPI) revokeIssuedTokens(until time.Time) error {
	issuedTokens, err := api.state.ListSecretBackendIssuedTokenUntil(until)
	if err != nil {
		return errors.Trace(err)
	}

	if len(issuedTokens) == 0 {
		return nil
	}

	issuedTokensToBackend := map[string][]string{}
	for _, ik := range issuedTokens {
		b := issuedTokensToBackend[ik.BackendID]
		b = append(b, ik.UUID)
		issuedTokensToBackend[ik.BackendID] = b
	}

	adminCfg, err := api.backendConfigGetter()
	if err != nil {
		return errors.Trace(err)
	}

	for backendID, issuedTokenUUIDs := range issuedTokensToBackend {
		backendCfg, ok := adminCfg.Configs[backendID]
		if !ok {
			// If the backend doesn't exist. Discard the tokens.
			err = api.state.RemoveSecretBackendIssuedTokens(issuedTokenUUIDs)
			if err != nil {
				return errors.Trace(err)
			}
			continue
		}

		p, err := api.providerGetter(backendCfg.BackendType)
		if err != nil {
			return errors.Trace(err)
		}

		removedUUIDs, cleanUpErr := p.CleanupIssuedTokens(
			&backendCfg, issuedTokenUUIDs)
		if len(removedUUIDs) > 0 {
			err = api.state.RemoveSecretBackendIssuedTokens(issuedTokenUUIDs)
			if err != nil {
				return errors.Trace(err)
			}
		}
		if cleanUpErr != nil {
			return errors.Trace(cleanUpErr)
		}
	}

	return nil
}
