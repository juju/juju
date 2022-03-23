// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/state/stateenvirons"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
	registry.MustRegister("SSHClient", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*FacadeV2)(nil)))
	registry.MustRegister("SSHClient", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx) // v2 adds AllAddresses() method.
	}, reflect.TypeOf((*FacadeV2)(nil)))
	registry.MustRegister("SSHClient", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx) // v3 adds Leader() method.
	}, reflect.TypeOf((*Facade)(nil)))
}

func newFacade(ctx facade.Context) (*Facade, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	leadershipReader, err := ctx.LeadershipReader(m.UUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalFacade(
		&backend{st, stateenvirons.EnvironConfigGetter{Model: m}, m.ModelTag()},
		leadershipReader,
		ctx.Auth(),
		context.CallContext(st))
}

// newFacadeV2 is used for API registration.
func newFacadeV2(ctx facade.Context) (*FacadeV2, error) {
	f, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &FacadeV2{Facade: f}, nil
}
