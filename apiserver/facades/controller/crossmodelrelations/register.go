// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CrossModelRelations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPIV2(ctx) // Adds WatchRelationChanges, removes WatchRelationUnits
	}, reflect.TypeOf((*CrossModelRelationsAPIV2)(nil)))
	registry.MustRegister("CrossModelRelations", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPI(ctx) // Adds WatchRelationChanges, removes WatchRelationUnits
	}, reflect.TypeOf((*CrossModelRelationsAPI)(nil)))
}

// newStateCrossModelRelationsAPI creates a new server-side CrossModelRelations API facade
// backed by global state.
func newStateCrossModelRelationsAPIV2(ctx facade.Context) (*CrossModelRelationsAPIV2, error) {
	api, err := newStateCrossModelRelationsAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &CrossModelRelationsAPIV2{api}, nil
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
		watchConsumedSecrets,
	)
}
