// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// SecretBackendsAPI is the server implementation for the SecretBackends facade.
type SecretBackendsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string

	state SecretsBackendState
}

func (s *SecretBackendsAPI) checkCanAdmin() error {
	canAdmin, err := s.authorizer.HasPermission(permission.SuperuserAccess, names.NewControllerTag(s.controllerUUID))
	if err != nil {
		return errors.Trace(err)
	}
	if !canAdmin {
		return apiservererrors.ErrPerm
	}
	return nil
}

// AddSecretBackends adds new secret backends.
func (s *SecretBackendsAPI) AddSecretBackends(args params.AddSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		// TODO(wallyworld) - add backend specific config validation
		err := s.state.AddSecretBackend(state.CreateSecretBackendParams{
			Name:                arg.Name,
			Backend:             arg.Backend,
			TokenRotateInterval: arg.TokenRotateInterval,
			Config:              arg.Config,
		})
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

// ListSecretBackends lists available secret backends.
func (s *SecretBackendsAPI) ListSecretBackends(arg params.ListSecretBackendsArgs) (params.ListSecretBackendsResults, error) {
	result := params.ListSecretBackendsResults{}
	if arg.Reveal {
		if err := s.checkCanAdmin(); err != nil {
			return result, errors.Trace(err)
		}
	}
	backends, err := s.state.ListSecretBackends()
	if err != nil {
		return params.ListSecretBackendsResults{}, errors.Trace(err)
	}
	result.Results = make([]params.SecretBackend, len(backends))
	for i, b := range backends {
		// TODO(wallyworld) - filter out tokens etc if reveal == false
		backend := params.SecretBackend{
			Name:                b.Name,
			Backend:             b.Backend,
			TokenRotateInterval: b.TokenRotateInterval,
			Config:              b.Config,
		}
		result.Results[i] = backend
	}
	return result, nil
}
