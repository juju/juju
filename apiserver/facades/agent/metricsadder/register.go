// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsadder

import (
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
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
