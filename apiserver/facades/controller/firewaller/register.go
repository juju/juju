// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Firewaller", 7, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newFirewallerAPIV7(ctx)
	}, reflect.TypeOf((*FirewallerAPI)(nil)))
}

// newFirewallerAPIV7 creates a new server-side FirewallerAPIv7 facade.
func newFirewallerAPIV7(ctx facade.ModelContext) (*FirewallerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	domainServices := ctx.DomainServices()
	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st, domainServices.Cloud(), domainServices.Credential(), domainServices.Config()),
		cloudspec.MakeCloudSpecWatcherForModel(st, domainServices.Cloud()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st, domainServices.Credential()),
		common.AuthFuncForTag(m.ModelTag()),
	)
	controllerConfigAPI := common.NewControllerConfigAPI(
		st,
		domainServices.ControllerConfig(),
		domainServices.ExternalController(),
	)

	stShim := stateShim{st: st, State: firewall.StateShim(st, m), MacaroonGetter: crossmodel.GetBackend(st)}
	return NewStateFirewallerAPI(
		stShim,
		domainServices.Network(),
		ctx.Resources(),
		ctx.WatcherRegistry(),
		ctx.Auth(),
		cloudSpecAPI,
		controllerConfigAPI,
		domainServices.ControllerConfig(),
		domainServices.Config(),
		domainServices.Application(),
		domainServices.Machine(),
		ctx.Logger().Child("firewaller"),
	)
}
