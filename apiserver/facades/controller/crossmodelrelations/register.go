// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/v2/apiserver/common"
	commoncrossmodel "github.com/juju/juju/v2/apiserver/common/crossmodel"
	"github.com/juju/juju/v2/apiserver/common/firewall"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossModelRelations", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPIV1(ctx)
	}, reflect.TypeOf((*CrossModelRelationsAPIV1)(nil)))
	registry.MustRegister("CrossModelRelations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPI(ctx) // Adds WatchRelationChanges, removes WatchRelationUnits
	}, reflect.TypeOf((*CrossModelRelationsAPI)(nil)))
}

// newStateCrossModelRelationsAPIV1 creates a new server-side
// CrossModelRelations v1 API facade backed by state.
func newStateCrossModelRelationsAPIV1(ctx facade.Context) (*CrossModelRelationsAPIV1, error) {
	api, err := newStateCrossModelRelationsAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CrossModelRelationsAPIV1{api}, nil
}

// newStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossModelRelationsAPI(ctx facade.Context) (*CrossModelRelationsAPI, error) {
	authCtxt := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value
	st := ctx.State()
	model, err := st.Model()
	if err != nil {
		return nil, err
	}

	return NewCrossModelRelationsAPI(
		stateShim{
			st:      st,
			Backend: commoncrossmodel.GetBackend(st),
		},
		firewall.StateShim(st, model),
		ctx.Resources(), ctx.Auth(),
		authCtxt.(*commoncrossmodel.AuthContext),
		firewall.WatchEgressAddressesForRelations,
		watchRelationLifeSuspendedStatus,
		watchOfferStatus,
	)
}
