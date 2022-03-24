// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Cloud", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*CloudAPIV1)(nil)))
	registry.MustRegister("Cloud", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx) // adds AddCloud, AddCredentials, CredentialContents, RemoveClouds
	}, reflect.TypeOf((*CloudAPIV2)(nil)))
	registry.MustRegister("Cloud", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx) // changes signature of UpdateCredentials, adds ModifyCloudAccess
	}, reflect.TypeOf((*CloudAPIV3)(nil)))
	registry.MustRegister("Cloud", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx) // adds UpdateCloud
	}, reflect.TypeOf((*CloudAPIV4)(nil)))
	registry.MustRegister("Cloud", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx) // Removes DefaultCloud, handles config in AddCloud
	}, reflect.TypeOf((*CloudAPIV5)(nil)))
	registry.MustRegister("Cloud", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx) // Adds validity to CredentialContent, force for AddCloud
	}, reflect.TypeOf((*CloudAPIV6)(nil)))
	registry.MustRegister("Cloud", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx) // Do not set error if forcing credential update.
	}, reflect.TypeOf((*CloudAPI)(nil)))
}

// newFacadeV7 is used for API registration.
func newFacadeV7(context facade.Context) (*CloudAPI, error) {
	st := NewStateBackend(context.State())
	pool := NewModelPoolBackend(context.StatePool())
	ctlrSt := NewStateBackend(pool.SystemState())
	return NewCloudAPI(st, ctlrSt, pool, context.Auth())
}

// newFacadeV6 is used for API registration.
func newFacadeV6(context facade.Context) (*CloudAPIV6, error) {
	v6, err := newFacadeV7(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV6{v6}, nil
}

// newFacadeV5 is used for API registration.
func newFacadeV5(context facade.Context) (*CloudAPIV5, error) {
	v6, err := newFacadeV6(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV5{v6}, nil
}

// newFacadeV4 is used for API registration.
func newFacadeV4(context facade.Context) (*CloudAPIV4, error) {
	v5, err := newFacadeV5(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV4{v5}, nil
}

// newFacadeV3 is used for API registration.
func newFacadeV3(context facade.Context) (*CloudAPIV3, error) {
	v4, err := newFacadeV4(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV3{v4}, nil
}

// newFacadeV2 is used for API registration.
func newFacadeV2(context facade.Context) (*CloudAPIV2, error) {
	v3, err := newFacadeV3(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV2{v3}, nil
}

// newFacadeV1 is used for API registration.
func newFacadeV1(context facade.Context) (*CloudAPIV1, error) {
	v2, err := newFacadeV2(context)
	if err != nil {
		return nil, err
	}
	return &CloudAPIV1{v2}, nil
}
