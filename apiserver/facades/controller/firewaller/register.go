// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Firewaller", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFirewallerAPIV7(ctx)
	}, reflect.TypeOf((*FirewallerAPI)(nil)))
}

// newFirewallerAPIV7 creates a new server-side FirewallerAPIv7 facade.
func newFirewallerAPIV7(ctx facade.Context) (*FirewallerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		ctx.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)
	serviceFactory := ctx.ServiceFactory()
	controllerConfigAPI := common.NewControllerConfigAPI(
		st,
		serviceFactory.ControllerConfig(),
		serviceFactory.ExternalController(),
	)

	stShim := stateShim{st: st, State: firewall.StateShim(st, m)}
	return NewStateFirewallerAPI(
		stShim,
		ctx.Resources(),
		ctx.Auth(),
		cloudSpecAPI,
		controllerConfigAPI,
		ctx.Logger().Child("firewaller"),
	)
}
