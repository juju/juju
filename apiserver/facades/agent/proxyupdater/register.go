// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package proxyupdater

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ProxyUpdater", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*APIv1)(nil)))
	registry.MustRegister("ProxyUpdater", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
}

// newFacadeV1 provides the signature required for facade registration
// and creates a v1 facade.
func newFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := newFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// newFacadeV2 provides the signature required for facade registration
// and creates a v2 facade.
func newFacadeV2(ctx facade.Context) (*APIv2, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}
