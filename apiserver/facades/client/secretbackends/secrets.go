// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/secrets/provider"
	_ "github.com/juju/juju/secrets/provider/all"
	"github.com/juju/juju/secrets/provider/juju"
	"github.com/juju/juju/state"
)

// SecretBackendsAPI is the server implementation for the SecretBackends facade.
type SecretBackendsAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string

	backendState SecretsBackendState
	secretState  SecretsState
	statePool    StatePool
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
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.createBackend(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretBackendsAPI) createBackend(arg params.SecretBackend) error {
	if arg.Name == "" {
		return errors.NotValidf("missing backend name")
	}
	if arg.Name == juju.BackendName || arg.Name == provider.Auto {
		return errors.NotValidf("backend %q")
	}
	p, err := provider.Provider(arg.BackendType)
	if err != nil {
		return errors.Annotatef(err, "creating backend provider type %q", arg.BackendType)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		err = configValidator.ValidateConfig(nil, arg.Config)
		if err != nil {
			return errors.Annotatef(err, "invalid config for provider %q", arg.BackendType)
		}
	}
	return s.backendState.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                arg.Name,
		Backend:             arg.BackendType,
		TokenRotateInterval: arg.TokenRotateInterval,
		Config:              arg.Config,
	})
}

// ListSecretBackends lists available secret backends.
func (s *SecretBackendsAPI) ListSecretBackends(arg params.ListSecretBackendsArgs) (params.ListSecretBackendsResults, error) {
	result := params.ListSecretBackendsResults{}
	if arg.Reveal {
		if err := s.checkCanAdmin(); err != nil {
			return result, errors.Trace(err)
		}
	}

	results, err := commonsecrets.BackendSummaryInfo(s.statePool, s.backendState, s.secretState, s.controllerUUID, arg.Reveal, true)
	if err != nil {
		return params.ListSecretBackendsResults{}, errors.Trace(err)
	}
	result.Results = results
	return result, nil
}

// RemoveSecretBackends removes secret backends.
func (s *SecretBackendsAPI) RemoveSecretBackends(args params.RemoveSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.removeBackend(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretBackendsAPI) removeBackend(arg params.RemoveSecretBackendArg) error {
	if arg.Name == "" {
		return errors.NotValidf("missing backend name")
	}
	if arg.Name == juju.BackendName || arg.Name == provider.Auto {
		return errors.NotValidf("backend %q")
	}
	return s.backendState.DeleteSecretBackend(arg.Name, arg.Force)
}
