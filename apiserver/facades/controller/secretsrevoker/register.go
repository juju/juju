// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package secretsrevoker

import (
	"reflect"

	"github.com/juju/errors"

	commonsecrets "github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	secretsprovider "github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("SecretsRevoker", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newSecretsRevokerAPI(ctx)
	}, reflect.TypeOf((*SecretsRevokerAPI)(nil)))
}

// newSecretsRevokerAPI creates a SecretsRevokerAPI for revoking secret backend
// tokens.
func newSecretsRevokerAPI(context facade.Context) (*SecretsRevokerAPI, error) {
	if !context.Auth().AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	model, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendConfigGetter := func() (*secretsprovider.ModelBackendConfigInfo, error) {
		return commonsecrets.AdminBackendConfigInfo(commonsecrets.SecretsModel(model))
	}

	return &SecretsRevokerAPI{
		resources: context.Resources(),
		state:     state.NewSecrets(context.State()),

		backendConfigGetter: secretBackendConfigGetter,
		providerGetter:      secretsprovider.Provider,
	}, nil
}
