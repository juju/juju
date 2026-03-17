// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Provisioner", 12, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newProvisionerAPIV12(stdCtx, ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeFor[*ProvisionerAPI]())
	registry.MustRegister("Provisioner", 11, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newProvisionerAPIV11(stdCtx, ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeFor[*ProvisionerAPIV11]())
}

// newProvisionerAPIV12 creates a new server-side Provisioner API facade.
func newProvisionerAPIV12(stdCtx context.Context, ctx facade.ModelContext) (*ProvisionerAPI, error) {
	api, err := MakeProvisionerAPI(stdCtx, ctx)
	return api, errors.Trace(err)
}

// newProvisionerAPIV11 creates a new server-side Provisioner API facade
// for version 11.
func newProvisionerAPIV11(stdCtx context.Context, ctx facade.ModelContext) (*ProvisionerAPIV11, error) {
	api, err := MakeProvisionerAPI(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV11{ProvisionerAPI: api}, nil
}
