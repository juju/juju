// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "Provisioner",
		Versions: facades.FacadeVersion{11},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Provisioner", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV11(ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeOf((*ProvisionerAPIV11)(nil)))
}

// newProvisionerAPIV11 creates a new server-side Provisioner API facade.
func newProvisionerAPIV11(ctx facade.Context) (*ProvisionerAPIV11, error) {
	provisionerAPI, err := NewProvisionerAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV11{provisionerAPI}, nil
}
