// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usersecretsdrain

import (
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("UserSecretsDrain", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newUserSecretsDrainAPI(ctx)
	}, reflect.TypeOf((*commonsecrets.SecretsDrainAPI)(nil)))
}

// newUserSecretsDrainAPI creates a SecretsDrainAPI for draining user secrets.
func newUserSecretsDrainAPI(context facade.Context) (*commonsecrets.SecretsDrainAPI, error) {
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
	return commonsecrets.NewSecretsDrainAPI(
		context.Auth().GetAuthTag(),
		context.Auth(),
		context.Resources(),
		leadershipChecker,
		commonsecrets.SecretsModel(model),
		state.NewSecrets(context.State()),
		context.State(),
	)
}
