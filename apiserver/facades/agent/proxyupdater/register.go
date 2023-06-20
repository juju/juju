// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"reflect"

	"github.com/juju/errors"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ProxyUpdater", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*API)(nil)))
}

// newFacadeV2 provides the signature required for facade registration
// and creates a v2 facade.
func newFacadeV2(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !(authorizer.AuthMachineAgent() || authorizer.AuthUnitAgent() || authorizer.AuthApplicationAgent() || authorizer.AuthModelAgent()) {
		return nil, apiservererrors.ErrPerm
	}
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api, nil
}
