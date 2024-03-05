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
	registry.MustRegister("Provisioner", 11, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newProvisionerAPIV11(stdCtx, ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeOf((*ProvisionerAPIV11)(nil)))
}

// newProvisionerAPIV11 creates a new server-side Provisioner API facade.
func newProvisionerAPIV11(stdCtx context.Context, ctx facade.ModelContext) (*ProvisionerAPIV11, error) {
	provisionerAPI, err := NewProvisionerAPI(stdCtx, ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV11{provisionerAPI}, nil
}
