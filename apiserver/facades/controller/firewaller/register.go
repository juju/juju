// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/cloudspec"
	"github.com/juju/juju/apiserver/common/firewall"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Firewaller", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFirewallerAPIV3(ctx)
	}, reflect.TypeOf((*FirewallerAPIV3)(nil)))
	registry.MustRegister("Firewaller", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFirewallerAPIV4(ctx)
	}, reflect.TypeOf((*FirewallerAPIV4)(nil)))
	registry.MustRegister("Firewaller", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFirewallerAPIV5(ctx)
	}, reflect.TypeOf((*FirewallerAPIV5)(nil)))
	registry.MustRegister("Firewaller", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFirewallerAPIV6(ctx)
	}, reflect.TypeOf((*FirewallerAPIV6)(nil)))
	registry.MustRegister("Firewaller", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFirewallerAPIV7(ctx)
	}, reflect.TypeOf((*FirewallerAPIV7)(nil)))
}

// newStateFirewallerAPIV3 creates a new server-side FirewallerAPIV3 facade.
func newStateFirewallerAPIV3(context facade.Context) (*FirewallerAPIV3, error) {
	st := context.State()

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudSpecAPI := cloudspec.NewCloudSpecV1(
		context.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(st),
		cloudspec.MakeCloudSpecWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(st),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(st),
		common.AuthFuncForTag(m.ModelTag()),
	)
	return NewFirewallerAPI(stateShim{st: st, State: firewall.StateShim(st, m)}, context.Resources(), context.Auth(), cloudSpecAPI)
}

// newStateFirewallerAPIV4 creates a new server-side FirewallerAPIV4 facade.
func newStateFirewallerAPIV4(context facade.Context) (*FirewallerAPIV4, error) {
	facadev3, err := newStateFirewallerAPIV3(context)
	if err != nil {
		return nil, err
	}
	return &FirewallerAPIV4{
		FirewallerAPIV3: facadev3,
	}, nil
}

// newStateFirewallerAPIV5 creates a new server-side FirewallerAPIV5 facade.
func newStateFirewallerAPIV5(context facade.Context) (*FirewallerAPIV5, error) {
	facadev4, err := newStateFirewallerAPIV4(context)
	if err != nil {
		return nil, err
	}
	return &FirewallerAPIV5{
		FirewallerAPIV4: facadev4,
	}, nil
}

// newStateFirewallerAPIV6 creates a new server-side FirewallerAPIV6 facade.
func newStateFirewallerAPIV6(context facade.Context) (*FirewallerAPIV6, error) {
	facadev5, err := newStateFirewallerAPIV5(context)
	if err != nil {
		return nil, err
	}
	return &FirewallerAPIV6{
		FirewallerAPIV5: facadev5,
	}, nil
}

// newStateFirewallerAPIV7 creates a new server-side FirewallerAPIv7 facade.
func newStateFirewallerAPIV7(context facade.Context) (*FirewallerAPIV7, error) {
	facadev6, err := newStateFirewallerAPIV6(context)
	if err != nil {
		return nil, err
	}
	m, err := context.State().Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	cloudSpecAPI := cloudspec.NewCloudSpecV2(
		context.Resources(),
		cloudspec.MakeCloudSpecGetterForModel(context.State()),
		cloudspec.MakeCloudSpecWatcherForModel(context.State()),
		cloudspec.MakeCloudSpecCredentialWatcherForModel(context.State()),
		cloudspec.MakeCloudSpecCredentialContentWatcherForModel(context.State()),
		common.AuthFuncForTag(m.ModelTag()),
	)
	facadev6.FirewallerAPIV3.CloudSpecer = cloudSpecAPI
	return &FirewallerAPIV7{
		FirewallerAPIV6: facadev6,
	}, nil
}
