// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
)

// SecretsManagerAPI is the backend for the SecretsManager facade.
type SecretsManagerAPI struct {
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &SecretsManagerAPI{}, nil
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(arg params.CreateSecretArg) (params.StringResults, error) {
	return params.StringResults{}, apiservererrors.ServerError(errors.NotImplementedf("CreateSecrets"))
}

// GetSecretValues returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretValues(arg params.GetSecretArgs) (params.SecretValueResults, error) {
	return params.SecretValueResults{}, apiservererrors.ServerError(errors.NotImplementedf("GetSecretValues"))
}
