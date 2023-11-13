// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperator

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common/unitcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASOperator", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*Facade)(nil)))
}

// newStateFacade provides the signature required for facade registration.
func newStateFacade(ctx facade.Context) (*Facade, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caasBroker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model)
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	leadershipRevoker, err := ctx.LeadershipRevoker(ctx.State().ModelUUID())
	if err != nil {
		return nil, errors.Annotate(err, "getting leadership client")
	}
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewFacade(resources, authorizer,
		stateShim{systemState},
		stateShim{ctx.State()},
		unitcommon.Backend(ctx.State()),
		caasBroker, leadershipRevoker)
}
