// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASOperatorUpgrader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateCAASOperatorUpgraderAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newStateCAASOperatorUpgraderAPI provides the signature required for facade registration.
func newStateCAASOperatorUpgraderAPI(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	model, err := ctx.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(model, ctx.ServiceFactory().Cloud(), ctx.ServiceFactory().Credential())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	return NewCAASOperatorUpgraderAPI(authorizer, broker, ctx.Logger().Child("caasoperatorupgrader"))
}
