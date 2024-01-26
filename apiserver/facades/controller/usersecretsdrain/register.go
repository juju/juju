// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsDrain", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newUserSecretsDrainAPI(ctx)
	}, reflect.TypeOf((*SecretsDrainAPI)(nil)))
}

// newUserSecretsDrainAPI creates a SecretsDrainAPI for draining user secrets.
func newUserSecretsDrainAPI(context facade.Context) (*SecretsDrainAPI, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	leadershipChecker, err := context.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	authTag := model.ModelTag()
	commonDrainAPI, err := commonsecrets.NewSecretsDrainAPI(
		authTag,
		context.Auth(),
		context.Resources(),
		leadershipChecker,
		commonsecrets.SecretsModel(model),
		state.NewSecrets(context.State()),
		context.State(),
	)
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendConfigGetter := func(backendIDs []string, wantAll bool) (*provider.ModelBackendConfigInfo, error) {
		return commonsecrets.BackendConfigInfo(commonsecrets.SecretsModel(model), true, backendIDs, wantAll, authTag, leadershipChecker)
	}
	secretBackendDrainConfigGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return commonsecrets.DrainBackendConfigInfo(backendID, commonsecrets.SecretsModel(model), authTag, leadershipChecker)
	}

	return &SecretsDrainAPI{
		SecretsDrainAPI:     commonDrainAPI,
		drainConfigGetter:   secretBackendDrainConfigGetter,
		backendConfigGetter: secretBackendConfigGetter,
		secretsState:        state.NewSecrets(context.State()),
	}, nil
}
