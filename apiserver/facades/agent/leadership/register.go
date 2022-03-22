// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leadership

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
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
	registry.MustRegister("LeadershipService", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newLeadershipServiceFacade(ctx)
	}, reflect.TypeOf((*LeadershipService)(nil)))
}

// newLeadershipServiceFacade constructs a new LeadershipService and presents
// a signature that can be used for facade registration.
func newLeadershipServiceFacade(context facade.Context) (LeadershipService, error) {
	claimer, err := context.LeadershipClaimer(context.State().ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewLeadershipService(claimer, context.Auth())
}
