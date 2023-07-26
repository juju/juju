// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsdrain

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsDrain", 1, func(ctx facade.Context) (facade.Facade, error) {
		return NewSecretManagerAPI(ctx)
	}, reflect.TypeOf((*SecretsDrainAPI)(nil)))
}

// NewSecretManagerAPI creates a SecretsDrainAPI.
func NewSecretManagerAPI(context facade.Context) (*SecretsDrainAPI, error) {
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
	return &SecretsDrainAPI{
		authTag:           context.Auth().GetAuthTag(),
		leadershipChecker: leadershipChecker,
		secretsState:      state.NewSecrets(context.State()),
		watcherRegistry:   context.WatcherRegistry(),
		secretsConsumer:   context.State(),
		model:             model,
		logger:            context.Logger().Child("secretsdrain"),
	}, nil
}
