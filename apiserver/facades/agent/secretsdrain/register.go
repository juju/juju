// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

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
	registry.MustRegister("SecretsDrain", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newSecretsDrainAPI(ctx)
	}, reflect.TypeOf((*commonsecrets.SecretsDrainAPI)(nil)))
}

// newSecretsDrainAPI creates a SecretsDrainAPI.
func newSecretsDrainAPI(context facade.Context) (*commonsecrets.SecretsDrainAPI, error) {
	if !context.Auth().AuthUnitAgent() && !context.Auth().AuthApplicationAgent() {
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
	authTag := context.Auth().GetAuthTag()
	return commonsecrets.NewSecretsDrainAPI(
		authTag,
		context.Auth(),
		context.Resources(),
		leadershipChecker,
		commonsecrets.SecretsModel(model),
		state.NewSecrets(context.State()),
		context.State(),
	)
}
