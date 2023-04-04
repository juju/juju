// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secrets

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Secrets", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newSecretsAPI(ctx)
	}, reflect.TypeOf((*SecretsAPI)(nil)))
}

// newSecretsAPI creates a SecretsAPI.
func newSecretsAPI(context facade.Context) (*SecretsAPI, error) {
	if !context.Auth().AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	backend := state.NewSecrets(context.State())

	secretGetter := func() (provider.SecretsBackend, error) {
		model, err := context.State().Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return secrets.StoreForInspect(model)
	}
	return &SecretsAPI{
		authorizer:     context.Auth(),
		controllerUUID: context.State().ControllerUUID(),
		modelUUID:      context.State().ModelUUID(),
		state:          backend,
		storeGetter:    secretGetter,
	}, nil
}
