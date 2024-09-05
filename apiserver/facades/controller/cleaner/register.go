// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/secrets"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujusecrets "github.com/juju/juju/secrets"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cleaner", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newCleanerAPI(ctx)
	}, reflect.TypeOf((*CleanerAPI)(nil)))
}

// newCleanerAPI creates a new instance of the Cleaner API.
func newCleanerAPI(ctx facade.Context) (*CleanerAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}

	st := getState(ctx.State())
	m, err := st.SecretsModel()
	if err != nil {
		return nil, errors.Trace(err)
	}
	secretBackendConfigGetter := func(backendID string) (*provider.ModelBackendConfigInfo, error) {
		return secrets.SecretCleanupBackendConfigInfo(m, backendID)
	}
	backend := jujusecrets.NewClientForContentDeletion(state.NewSecrets(ctx.State()), secretBackendConfigGetter)

	return &CleanerAPI{
		st:                   st,
		resources:            ctx.Resources(),
		secretContentDeleter: backend.DeleteContent,
	}, nil
}
