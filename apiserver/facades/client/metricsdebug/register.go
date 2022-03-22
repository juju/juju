// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

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
	registry.MustRegister("MetricsDebug", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newMetricsDebugAPI(ctx)
	}, reflect.TypeOf((*MetricsDebugAPI)(nil)))
}

// newMetricsDebugAPI creates a new API endpoint for calling metrics debug functions.
func newMetricsDebugAPI(ctx facade.Context) (*MetricsDebugAPI, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &MetricsDebugAPI{
		state: ctx.State(),
	}, nil
}
