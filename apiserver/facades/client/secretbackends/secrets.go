// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretbackends

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/secrets"
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

	clock        clock.Clock
	backendState SecretsBackendState
	secretState  SecretsState
	statePool    StatePool
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
		err := s.createBackend(arg.ID, arg.SecretBackend)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (s *SecretBackendsAPI) createBackend(id string, arg params.SecretBackend) error {
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

	var nextRotateTime *time.Time
	if arg.TokenRotateInterval != nil && *arg.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *arg.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
		}
	}
	_, err = s.backendState.CreateSecretBackend(state.CreateSecretBackendParams{
		ID:                  id,
		Name:                arg.Name,
		BackendType:         arg.BackendType,
		TokenRotateInterval: arg.TokenRotateInterval,
		NextRotateTime:      nextRotateTime,
		Config:              arg.Config,
	})
	if errors.IsAlreadyExists(err) {
		return errors.AlreadyExistsf("secret backend with ID %q", id)
	}
	return errors.Trace(err)
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
		err = configValidator.ValidateConfig(existing.Config, cfg)
		if err != nil {
			return errors.Annotatef(err, "invalid config for provider %q", existing.BackendType)
		}
	}
	if !arg.Force {
		if err := commonsecrets.PingBackend(p, cfg); err != nil {
			return errors.Trace(err)
		}
	}
	var nextRotateTime *time.Time
	if arg.TokenRotateInterval != nil && *arg.TokenRotateInterval > 0 {
		if !provider.HasAuthRefresh(p) {
			return errors.NotSupportedf("token refresh on secret backend of type %q", p.Type())
		}
		nextRotateTime, err = secrets.NextBackendRotateTime(s.clock.Now(), *arg.TokenRotateInterval)
		if err != nil {
			return errors.Trace(err)
		}
	}
	err = s.backendState.UpdateSecretBackend(state.UpdateSecretBackendParams{
		ID:                  existing.ID,
		NameChange:          arg.NameChange,
		TokenRotateInterval: arg.TokenRotateInterval,
		NextRotateTime:      nextRotateTime,
		Config:              cfg,
	})
	if errors.IsNotFound(err) {
		return errors.NotFoundf("secret backend %q", arg.Name)
	}
	return err
}

// ListSecretBackends lists available secret backends.
func (s *SecretBackendsAPI) ListSecretBackends(ctx context.Context, arg params.ListSecretBackendsArgs) (params.ListSecretBackendsResults, error) {
	result := params.ListSecretBackendsResults{}
	if arg.Reveal {
		if err := s.checkCanAdmin(); err != nil {
			return result, errors.Trace(err)
		}
	}

	results, err := commonsecrets.BackendSummaryInfo(
		s.statePool, s.backendState, s.secretState, s.controllerUUID, arg.Reveal, commonsecrets.BackendFilter{Names: arg.Names, All: true})
	if err != nil {
		return params.ListSecretBackendsResults{}, errors.Trace(err)
	}
	result.Results = results
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
