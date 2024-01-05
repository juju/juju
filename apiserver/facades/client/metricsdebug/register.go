// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metricsdebug

import (
	"context"
	"reflect"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MetricsDebug", 2, func(stdCtx context.Context, ctx facade.Context) (facade.Facade, error) {
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
