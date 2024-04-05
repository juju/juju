// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"fmt"

	"github.com/juju/clock"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
	"github.com/juju/juju/domain/secretbackend"
	secretbackendservice "github.com/juju/juju/domain/secretbackend/service"
	"github.com/juju/juju/internal/secrets/provider"
	_ "github.com/juju/juju/internal/secrets/provider/all"
	"github.com/juju/juju/internal/secrets/provider/juju"
	"github.com/juju/juju/internal/uuid"
	"github.com/juju/juju/rpc/params"
)

// SecretBackendsAPI is the server implementation for the SecretBackends facade.
type SecretBackendsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string

	clock       clock.Clock
	secretState SecretsState
	statePool   StatePool
	model       Model

	backendService SecretsBackendService
}

func (s *SecretBackendsAPI) checkCanAdmin() error {
	return s.authorizer.HasPermission(permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID))
}

// AddSecretBackends adds new secret backends.
func (s *SecretBackendsAPI) AddSecretBackends(ctx context.Context, args params.AddSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		if arg.ID == "" {
			uuid, err := uuid.NewUUID()
			if err != nil {
				result.Results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			arg.ID = uuid.String()
		}
		err := s.backendService.CreateSecretBackend(ctx, secrets.SecretBackend{
			ID:                  arg.ID,
			Name:                arg.Name,
			BackendType:         arg.BackendType,
			TokenRotateInterval: arg.TokenRotateInterval,
			Config:              arg.Config,
		})
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// UpdateSecretBackends updates secret backends.
func (s *SecretBackendsAPI) UpdateSecretBackends(ctx context.Context, args params.UpdateSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		params := secretbackendservice.UpdateSecretBackendParams{
			UpdateSecretBackendParams: secretbackend.UpdateSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{
					Name: arg.Name,
				},
				NewName:             arg.NameChange,
				TokenRotateInterval: arg.TokenRotateInterval,
			},
			SkipPing: arg.Force,
			Reset:    arg.Reset,
		}
		if len(arg.Config) > 0 {
			params.Config = transform.Map(arg.Config, func(k string, v interface{}) (string, string) {
				return k, fmt.Sprintf("%v", v)
			})
		}
		err := s.backendService.UpdateSecretBackend(ctx, params)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ListSecretBackends lists available secret backends.
func (s *SecretBackendsAPI) ListSecretBackends(ctx context.Context, arg params.ListSecretBackendsArgs) (params.ListSecretBackendsResults, error) {
	if arg.Reveal {
		if err := s.checkCanAdmin(); err != nil {
			return params.ListSecretBackendsResults{}, errors.Trace(err)
		}
	}

	backends, err := s.backendService.BackendSummaryInfo(ctx, arg.Reveal, true, arg.Names...)
	if err != nil {
		return params.ListSecretBackendsResults{}, errors.Trace(err)
	}
	result := params.ListSecretBackendsResults{
		Results: make([]params.SecretBackendResult, len(backends)),
	}
	for i, backend := range backends {
		result.Results[i] = params.SecretBackendResult{
			ID:         backend.ID,
			NumSecrets: backend.NumSecrets,
			Status:     backend.Status,
			Message:    backend.Message,
			Result: params.SecretBackend{
				Name:                backend.Name,
				BackendType:         backend.BackendType,
				TokenRotateInterval: backend.TokenRotateInterval,
				Config:              backend.Config,
			},
		}
	}
	return result, nil
}

// RemoveSecretBackends removes secret backends.
func (s *SecretBackendsAPI) RemoveSecretBackends(ctx context.Context, args params.RemoveSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		if arg.Name == "" {
			err := errors.NotValidf("missing backend name")
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if arg.Name == juju.BackendName || arg.Name == provider.Auto {
			err := errors.NotValidf("backend %q", arg.Name)
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err := s.backendService.DeleteSecretBackend(ctx,
			secretbackendservice.DeleteSecretBackendParams{
				BackendIdentifier: secretbackend.BackendIdentifier{Name: arg.Name},
				Force:             arg.Force,
			})
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}
