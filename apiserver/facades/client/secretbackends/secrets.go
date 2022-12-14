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
		err := s.createBackend(arg.SecretBackend)
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
	p, err := commonsecrets.GetProvider(arg.BackendType)
	if err != nil {
		return errors.Annotatef(err, "creating backend provider type %q", arg.BackendType)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		defaults := configValidator.ConfigDefaults()
		if arg.Config == nil && len(defaults) > 0 {
			arg.Config = make(map[string]interface{})
		}
		for k, v := range defaults {
			if _, ok := arg.Config[k]; !ok {
				arg.Config[k] = v
			}
		}
		err = configValidator.ValidateConfig(nil, arg.Config)
		if err != nil {
			return errors.Annotatef(err, "invalid config for provider %q", arg.BackendType)
		}
	}
	if err := commonsecrets.PingBackend(p, arg.Config); err != nil {
		return errors.Trace(err)
	}
	return s.backendState.CreateSecretBackend(state.CreateSecretBackendParams{
		Name:                arg.Name,
		BackendType:         arg.BackendType,
		TokenRotateInterval: arg.TokenRotateInterval,
		Config:              arg.Config,
	})
}

// UpdateSecretBackends updates secret backends.
func (s *SecretBackendsAPI) UpdateSecretBackends(args params.UpdateSecretBackendArgs) (params.ErrorResults, error) {
	result := params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Args)),
	}
	if err := s.checkCanAdmin(); err != nil {
		return result, errors.Trace(err)
	}
	for i, arg := range args.Args {
		err := s.updateBackend(arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretBackendsAPI) updateBackend(arg params.UpdateSecretBackendArg) error {
	if arg.Name == "" {
		return errors.NotValidf("missing backend name")
	}
	if arg.Name == juju.BackendName || arg.Name == provider.Auto {
		return errors.NotValidf("backend %q")
	}
	existing, err := s.backendState.GetSecretBackend(arg.Name)
	if err != nil {
		return errors.Trace(err)
	}
	p, err := commonsecrets.GetProvider(existing.BackendType)
	if err != nil {
		return errors.Trace(err)
	}

	cfg := make(map[string]interface{})
	for k, v := range existing.Config {
		cfg[k] = v
	}
	for k, v := range arg.Config {
		cfg[k] = v
	}
	for _, k := range arg.Reset {
		delete(cfg, k)
	}
	configValidator, ok := p.(provider.ProviderConfig)
	if ok {
		defaults := configValidator.ConfigDefaults()
		for _, k := range arg.Reset {
			if defaultVal, ok := defaults[k]; ok {
				cfg[k] = defaultVal
			}
		}
		err = configValidator.ValidateConfig(nil, cfg)
		if err != nil {
			return errors.Annotatef(err, "invalid config for provider %q", existing.BackendType)
		}
	}
	if !arg.Force {
		if err := commonsecrets.PingBackend(p, cfg); err != nil {
			return errors.Trace(err)
		}
	}
	err = s.backendState.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  existing.ID,
		NameChange:          arg.NameChange,
		TokenRotateInterval: arg.TokenRotateInterval,
		Config:              cfg,
	})
	if errors.IsNotFound(err) {
		return errors.NotFoundf("secret backend %q", arg.Name)
	}
	return err
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
