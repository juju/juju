// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ApplicationOffers", 5, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return makeOffersAPI(stdCtx, ctx)
	}, reflect.TypeOf((*OffersAPIv5)(nil)))
}

// makeOffersAPI returns a new application offers OffersAPI facade.
func makeOffersAPI(ctx context.Context, facadeContext facade.ModelContext) (*OffersAPIv5, error) {
	serviceFactory := facadeContext.ServiceFactory()
	st := facadeContext.State()
	getControllerInfo := func(ctx context.Context) ([]string, string, error) {
		return common.ControllerAPIInfo(ctx, st, serviceFactory.ControllerConfig())
	}

	authContext := facadeContext.Resources().Get("offerAccessAuthContext").(*common.ValueResource).Value
	return createOffersAPI(
		GetApplicationOffers,
		getControllerInfo,
		GetStateAccess(st),
		GetStatePool(facadeContext.StatePool()),
		serviceFactory.Model(),
		facadeContext.Auth(),
		authContext.(*commoncrossmodel.AuthContext),
		facadeContext.DataDir(),
		facadeContext.Logger().Child("applicationoffers"),
	)
}
