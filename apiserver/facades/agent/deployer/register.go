// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"reflect"

	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Deployer", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*DeployerAPI)(nil)))
}

// newAPI creates a new server-side DeployerAPI facade.
func newAPI(ctx facade.Context) (*DeployerAPI, error) {
	return NewDeployerAPI(ctx)
}
