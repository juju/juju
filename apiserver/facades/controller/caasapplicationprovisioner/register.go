// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplicationprovisioner

import (
	"reflect"

	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CAASApplicationProvisioner", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newAPI(ctx)
	}, reflect.TypeOf((*APIGroup)(nil)))
}

// newAPI provides the signature required for facade registration.
func newAPI(ctx facade.Context) (*APIGroup, error) {
	return NewStateCAASApplicationProvisionerAPI(ctx)
}
