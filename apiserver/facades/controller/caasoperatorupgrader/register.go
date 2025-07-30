// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasoperatorupgrader

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/caas"
	"github.com/juju/juju/state/stateenvirons"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASOperatorUpgrader", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateCAASOperatorUpgraderAPI(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newStateCAASOperatorUpgraderAPI provides the signature required for facade registration.
func newStateCAASOperatorUpgraderAPI(ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	domainServices := ctx.DomainServices()
	broker, err := stateenvirons.GetNewCAASBrokerFunc(caas.New)(domainServices.ModelInfo(), domainServices.Cloud(), domainServices.Credential(), domainServices.Config())
	if err != nil {
		return nil, errors.Annotate(err, "getting caas client")
	}
	return NewCAASOperatorUpgraderAPI(authorizer, broker, ctx.Logger().Child("caasoperatorupgrader"))
}
