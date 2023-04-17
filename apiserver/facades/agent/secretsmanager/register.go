// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsmanager

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewSecretManagerAPI(ctx)
	}, reflect.TypeOf((*SecretsManagerAPI)(nil)))
}

// NewSecretManagerAPI creates a SecretsManagerAPI.
func NewSecretManagerAPI(context facade.Context) (*SecretsManagerAPI, error) {
	if !context.Auth().AuthUnitAgent() && !context.Auth().AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		model, err := context.State().Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return secrets.BackendConfigInfo(secrets.SecretsModel(model), context.Auth().GetAuthTag(), leadershipChecker)
	}
	secretBackendAdminConfigGetter := func() (*provider.ModelBackendConfigInfo, error) {
		model, err := context.State().Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return secrets.AdminBackendConfigInfo(secrets.SecretsModel(model))
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &SecretsManagerAPI{
		authTag:             context.Auth().GetAuthTag(),
		leadershipChecker:   leadershipChecker,
		secretsState:        state.NewSecrets(context.State()),
		resources:           context.Resources(),
		secretsTriggers:     context.State(),
		secretsConsumer:     context.State(),
		clock:               clock.WallClock,
		backendConfigGetter: secretBackendConfigGetter,
		adminConfigGetter:   secretBackendAdminConfigGetter,
		model:               model,
	}, nil
}
