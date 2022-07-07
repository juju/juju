// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ApplicationOffers", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newOffersAPI(ctx)
	}, reflect.TypeOf((*OffersAPI)(nil)))
}

// newOffersAPI returns a new application offers OffersAPI facade.
func newOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	environFromModel := func(modelUUID string) (environs.Environ, error) {
		st, err := ctx.StatePool().Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		g := stateenvirons.EnvironConfigGetter{Model: model}
		env, err := environs.GetEnviron(g, environs.New)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return env, nil
	}

	st := ctx.State()
	getControllerInfo := func() ([]string, string, error) {
		return common.StateControllerInfo(st)
	}

	authContext := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value
	return createOffersAPI(
		GetApplicationOffers,
		environFromModel,
		getControllerInfo,
		GetStateAccess(st),
		GetStatePool(ctx.StatePool()),
		ctx.Auth(),
		ctx.Resources(),
		authContext.(*commoncrossmodel.AuthContext),
	)
}
