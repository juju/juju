// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelsecrets

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/internal/secrets/provider"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegisterForMultiModel("CrossModelSecrets", 1, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return newStateCrossModelSecretsAPI(stdCtx, ctx)
	}, reflect.TypeOf((*CrossModelSecretsAPI)(nil)))
}

// newStateCrossModelSecretsAPI creates a new server-side CrossModelSecrets API facade
// backed by global state.
func newStateCrossModelSecretsAPI(stdCtx context.Context, ctx facade.MultiModelContext) (*CrossModelSecretsAPI, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(*common.ValueResource).Value

	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	serviceFactory := ctx.ServiceFactory()

	backendService := serviceFactory.SecretBackend()
	secretBackendAdminConfigGetter := func(stdCtx context.Context) (*provider.ModelBackendConfigInfo, error) {
		return backendService.GetSecretBackendConfigForAdmin(stdCtx, coremodel.UUID(model.UUID()))
	}
	secretInfoGetter := func(modelUUID string) SecretService {
		return ctx.ServiceFactoryForModel(coremodel.UUID(modelUUID)).Secret(secretBackendAdminConfigGetter)
	}

	st := ctx.State()
	return NewCrossModelSecretsAPI(
		ctx.Resources(),
		authCtxt.(*crossmodel.AuthContext),
		st.ControllerUUID(),
		st.ModelUUID(),
		secretInfoGetter,
		backendService,
		&crossModelShim{st.RemoteEntities()},
		&stateBackendShim{st},
		ctx.Logger().ChildWithTags("crossmodelsecrets", corelogger.SECRETS),
	)
}
