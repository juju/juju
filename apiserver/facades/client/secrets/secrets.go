// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/secrets/provider/juju"
)

// SecretsAPI is the backend for the Secrets facade.
type SecretsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      string

	secretsService secrets.SecretsService
}

// NewSecretsAPI creates a SecretsAPI.
func NewSecretsAPI(context facade.Context) (*SecretsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	// For now we just support the Juju secrets provider.
	service, err := provider.NewSecretProvider(juju.Provider, secrets.ProviderConfig{
		juju.ParamBackend: context.State(),
	})
	if err != nil {
		return nil, errors.Annotate(err, "creating juju secrets service")
	}
	return &SecretsAPI{
		authorizer:     context.Auth(),
		controllerUUID: context.State().ControllerUUID(),
		modelUUID:      context.State().ModelUUID(),
		secretsService: service,
	}, nil
}

func (s *SecretsAPI) checkCanRead() error {
	canRead, err := s.authorizer.HasPermission(permission.ReadAccess, names.NewModelTag(s.modelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if !canRead {
		return apiservererrors.ErrPerm
	}
	return nil
}

func (s *SecretsAPI) checkCanAdmin() error {
	canAdmin, err := common.HasModelAdmin(s.authorizer, names.NewControllerTag(s.controllerUUID), names.NewModelTag(s.modelUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if !canAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

// ListSecrets lists available secrets.
func (s *SecretsAPI) ListSecrets(arg params.ListSecretsArgs) (params.ListSecretResults, error) {
	result := params.ListSecretResults{}
	if arg.ShowSecrets {
		if err := s.checkCanAdmin(); err != nil {
			return result, errors.Trace(err)
		}
	} else {
		if err := s.checkCanRead(); err != nil {
			return result, errors.Trace(err)
		}
	}
	ctx := context.Background()
	metadata, err := s.secretsService.ListSecrets(ctx, secrets.Filter{})
	if err != nil {
		return result, errors.Trace(err)
	}
	result.Results = make([]params.ListSecretResult, len(metadata))
	for i, m := range metadata {
		secretResult := params.ListSecretResult{
			Path:        m.Path,
			Scope:       string(m.Scope),
			Version:     m.Version,
			Description: m.Description,
			Tags:        m.Tags,
			ID:          m.ID,
			Provider:    m.Provider,
			ProviderID:  m.ProviderID,
			Revision:    m.Revision,
			CreateTime:  m.CreateTime,
			UpdateTime:  m.UpdateTime,
		}
		if arg.ShowSecrets {
			URL := &coresecrets.URL{
				Version:        fmt.Sprintf("v%d", m.Version),
				ControllerUUID: s.controllerUUID,
				ModelUUID:      s.modelUUID,
				Path:           m.Path,
			}
			val, err := s.secretsService.GetSecretValue(ctx, URL)
			valueResult := &params.SecretValueResult{
				Error: apiservererrors.ServerError(err),
			}
			if err == nil {
				valueResult.Data = val.EncodedValues()
			}
			secretResult.Value = valueResult
		}
		result.Results[i] = secretResult
	}
	return result, nil
}
