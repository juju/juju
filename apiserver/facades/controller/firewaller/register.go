// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/v3/apiserver/common"
	"github.com/juju/juju/v3/apiserver/common/cloudspec"
	"github.com/juju/juju/v3/apiserver/common/firewall"
	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Firewaller", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFirewallerAPIV7(ctx)
	}, reflect.TypeOf((*FirewallerAPI)(nil)))
}

// newFirewallerAPIV7 creates a new server-side FirewallerAPIv7 facade.
func newFirewallerAPIV7(context facade.Context) (*FirewallerAPI, error) {
	st := context.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		context.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)
	controllerConfigAPI := common.NewStateControllerConfig(st)

	stShim := stateShim{st: st, State: firewall.StateShim(st, m)}
	return NewStateFirewallerAPI(stShim, context.Resources(), context.Auth(), cloudSpecAPI, controllerConfigAPI)
}
