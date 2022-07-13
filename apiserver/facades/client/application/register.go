// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	// Application facade versions 1-4 share newFacadeV4 as
	// the newer methodology for versioning wasn't started with
	// Application until version 5.
	registry.MustRegister("Application", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Application", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Application", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Application", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*APIv4)(nil)))
	registry.MustRegister("Application", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx)
	}, reflect.TypeOf((*APIv5)(nil))) // adds AttachStorage & UpdateApplicationSeries & SetRelationStatus
	registry.MustRegister("Application", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*APIv6)(nil)))
	registry.MustRegister("Application", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*APIv7)(nil)))
	registry.MustRegister("Application", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV8(ctx)
	}, reflect.TypeOf((*APIv8)(nil)))
	registry.MustRegister("Application", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV9(ctx)
	}, reflect.TypeOf((*APIv9)(nil))) // ApplicationInfo; generational config; Force on App, Relation and Unit Removal.
	registry.MustRegister("Application", 10, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV10(ctx)
	}, reflect.TypeOf((*APIv10)(nil))) // --force and --no-wait parameters
	registry.MustRegister("Application", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV11(ctx)
	}, reflect.TypeOf((*APIv11)(nil))) // Get call returns the endpoint bindings
	registry.MustRegister("Application", 12, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV12(ctx)
	}, reflect.TypeOf((*APIv12)(nil))) // Adds UnitsInfo()
	registry.MustRegister("Application", 13, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV13(ctx)
	}, reflect.TypeOf((*APIv13)(nil))) // Adds Leader
	registry.MustRegister("Application", 14, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV14(ctx)
	}, reflect.TypeOf((*APIv14)(nil)))
}

// newFacadeV4 provides the signature required for facade registration
// for versions 1-4.
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
	api, err := newFacadeV7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv6{api}, nil
}

// newFacadeV7 provides the signature required for facade registration
// for version 7.
func newFacadeV7(ctx facade.Context) (*APIv7, error) {
	api, err := newFacadeV8(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv7{api}, nil
}

// newFacadeV8 provides the signature required for facade registration
// for version 8.
func newFacadeV8(ctx facade.Context) (*APIv8, error) {
	api, err := newFacadeV9(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv8{api}, nil
}

func newFacadeV9(ctx facade.Context) (*APIv9, error) {
	api, err := newFacadeV10(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv9{api}, nil
}

func newFacadeV10(ctx facade.Context) (*APIv10, error) {
	api, err := newFacadeV11(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv10{api}, nil
}

func newFacadeV11(ctx facade.Context) (*APIv11, error) {
	api, err := newFacadeV12(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv11{api}, nil
}

func newFacadeV12(ctx facade.Context) (*APIv12, error) {
	api, err := newFacadeV13(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv12{api}, nil
}

func newFacadeV13(ctx facade.Context) (*APIv13, error) {
	api, err := newFacadeV14(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv13{api}, nil
}

func newFacadeV14(ctx facade.Context) (*APIv14, error) {
	api, err := newFacadeBase(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv14{api}, nil
}
