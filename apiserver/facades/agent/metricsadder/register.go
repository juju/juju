// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
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
	registry.MustRegister("MetricsAdder", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newMetricsAdderAPI(ctx)
	}, reflect.TypeOf((*MetricsAdderAPI)(nil)))
}

// newMetricsAdderAPI creates a new API endpoint for adding metrics to state.
func newMetricsAdderAPI(ctx facade.Context) (*MetricsAdderAPI, error) {
	// TODO(cmars): remove unit agent auth, once worker/metrics/sender manifold
	// can be righteously relocated to machine agent.
	authorizer := ctx.Auth()
	if !authorizer.AuthMachineAgent() && !authorizer.AuthUnitAgent() {
		return nil, apiservererrors.ErrPerm
	}
	return &MetricsAdderAPI{
		state: ctx.State(),
	}, nil
}
