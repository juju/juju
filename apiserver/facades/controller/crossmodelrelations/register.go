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

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("CrossModelRelations", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPIV1(ctx)
	}, reflect.TypeOf((*CrossModelRelationsAPIV1)(nil)))
	registry.MustRegister("CrossModelRelations", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCrossModelRelationsAPI(ctx) // Adds WatchRelationChanges, removes WatchRelationUnits
	}, reflect.TypeOf((*CrossModelRelationsAPIV1)(nil)))
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
