// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/secrets"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/secrets/provider"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossModelSecrets", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelSecretsAPI(ctx)
	}, reflect.TypeOf((*CrossModelSecretsAPI)(nil)))
}

// newStateCrossModelSecretsAPI creates a new server-side CrossModelSecrets API facade
// backed by global state.
func newStateCrossModelSecretsAPI(ctx facade.Context) (*CrossModelSecretsAPI, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value

	leadershipChecker, err := ctx.LeadershipChecker()
	if err != nil {
		return nil, errors.Trace(err)
	}

	secretBackendConfigGetter := func(modelUUID, backendID string, consumer names.Tag) (*provider.ModelBackendConfigInfo, error) {
		model, closer, err := ctx.StatePool().GetModel(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer closer.Release()
		return secrets.BackendConfigInfo(secrets.SecretsModel(model), []string{backendID}, false, consumer, leadershipChecker)
	}
	secretInfoGetter := func(modelUUID string) (SecretsState, SecretsConsumer, func() bool, error) {
		st, err := ctx.StatePool().Get(modelUUID)
		if err != nil {
			return nil, nil, nil, errors.Trace(err)
		}
		return state.NewSecrets(st.State), st, st.Release, nil
	}

	st := ctx.State()
	return NewCrossModelSecretsAPI(
		ctx.Resources(),
		authCtxt.(*crossmodel.AuthContext),
		st.ModelUUID(),
		secretInfoGetter,
		secretBackendConfigGetter,
		&crossModelShim{st.RemoteEntities()},
		&stateBackendShim{st},
	)
}
