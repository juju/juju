// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Bundle", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*APIv6)(nil)))
}

// newFacadeV6 provides the signature required for facade registration
// for version 6.
func newFacadeV6(ctx facade.Context) (*APIv6, error) {
	api, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}
