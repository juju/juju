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
	registry.MustRegister("Bundle", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*APIv1)(nil)))
	registry.MustRegister("Bundle", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*APIv2)(nil)))
	registry.MustRegister("Bundle", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*APIv3)(nil)))
	registry.MustRegister("Bundle", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Bundle", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx)
	}, reflect.TypeOf((*APIv5)(nil)))
	registry.MustRegister("Bundle", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*APIv6)(nil)))
}

// newFacadeV1 provides the signature required for facade registration
// version 1.
func newFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := newFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &APIv1{api}, nil
}

// newFacadeV2 provides the signature required for facade registration
// for version 2.
func newFacadeV2(ctx facade.Context) (*APIv2, error) {
	api, err := newFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv2{api}, nil
}

// newFacadeV3 provides the signature required for facade registration
// for version 3.
func newFacadeV3(ctx facade.Context) (*APIv3, error) {
	api, err := newFacadeV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv3{api}, nil
}

// newFacadeV4 provides the signature required for facade registration
// for version 4.
func newFacadeV4(ctx facade.Context) (*APIv4, error) {
	api, err := newFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv4{api}, nil
}

// newFacadeV5 provides the signature required for facade registration
// for version 5.
func newFacadeV5(ctx facade.Context) (*APIv5, error) {
	api, err := newFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv5{api}, nil
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
