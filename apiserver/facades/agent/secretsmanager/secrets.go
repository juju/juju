// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"context"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
)

// SecretsManagerAPI is the backend for the SecretsManager facade.
type SecretsManagerAPI struct {
	controllerUUID string
	modelUUID      string

	secretsService secrets.SecretsService
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	// For now we just support the Juju secrets provider.
	service, err := provider.NewSecretProvider(juju.Provider, secrets.ProviderConfig{
		juju.ParamBackend: context.State(),
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating juju secrets service")
	}
	return &SecretsManagerAPI{
		controllerUUID: context.State().ControllerUUID(),
		modelUUID:      context.State().ModelUUID(),
		secretsService: service,
	}, nil
}

// CreateSecrets creates new secrets.
func (s *SecretsManagerAPI) CreateSecrets(args params.CreateSecretArgs) (params.StringResults, error) {
	result := params.StringResults{
		Results: make([]params.StringResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		ID, err := s.createSecret(ctx, arg)
		result.Results[i].Result = ID
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) createSecret(ctx context.Context, arg params.CreateSecretArg) (string, error) {
	url, _, err := s.secretsService.CreateSecret(ctx, secrets.CreateParams{
		ControllerUUID: s.controllerUUID,
		ModelUUID:      s.modelUUID,
		Version:        secrets.Version,
		Type:           arg.Type,
		Path:           arg.Path,
		Scope:          arg.Scope,
		Params:         arg.Params,
		Data:           arg.Data,
	})
	if err != nil {
		return "", errors.Trace(err)
	}
	return url.String(), nil
}

// GetSecretValues returns the secret values for the specified secrets.
func (s *SecretsManagerAPI) GetSecretValues(args params.GetSecretArgs) (params.SecretValueResults, error) {
	result := params.SecretValueResults{
		Results: make([]params.SecretValueResult, len(args.Args)),
	}
	ctx := context.Background()
	for i, arg := range args.Args {
		data, err := s.getSecretValue(ctx, arg)
		result.Results[i].Data = data
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretsManagerAPI) getSecretValue(ctx context.Context, arg params.GetSecretArg) (coresecrets.SecretData, error) {
	URL, err := coresecrets.ParseURL(arg.ID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if URL.ControllerUUID == "" {
		URL.ControllerUUID = s.controllerUUID
	}
	if URL.ModelUUID == "" {
		URL.ModelUUID = s.modelUUID
	}
	val, err := s.secretsService.GetSecretValue(ctx, URL)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return val.EncodedValues(), nil
}
