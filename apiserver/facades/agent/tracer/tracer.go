// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/internal"
	"github.com/juju/juju/rpc/params"
)

// TracerAPI implements the tracer API endpoint.
type TracerAPI struct {
	authorizer      facade.Authorizer
	watcherRegistry facade.WatcherRegistry

	controllerTracingConfigService ControllerTracingConfigService
}

// NewTracerAPI returns a TracerAPI facade.
func NewTracerAPI(
	authorizer facade.Authorizer,
	watcherRegistry facade.WatcherRegistry,
	controllerTracingConfigService ControllerTracingConfigService,
) (*TracerAPI, error) {
	if !authorizer.AuthMachineAgent() &&
		!authorizer.AuthUnitAgent() &&
		!authorizer.AuthApplicationAgent() &&
		!authorizer.AuthModelAgent() {
		return nil, apiservererrors.ErrPerm
	}

	return &TracerAPI{
		authorizer:                     authorizer,
		watcherRegistry:                watcherRegistry,
		controllerTracingConfigService: controllerTracingConfigService,
	}, nil
}

// GetControllerTracingConfig reports the controller-wide tracing
// configuration for the agent specified.
func (api *TracerAPI) GetControllerTracingConfig(ctx context.Context, arg params.Entity) params.TracingConfigResult {
	tag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return params.TracingConfigResult{Error: apiservererrors.ServerError(err)}
	}
	if !api.authorizer.AuthOwner(tag) {
		return params.TracingConfigResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	}

	config, err := api.controllerTracingConfigService.GetWorkloadTracingConfig(ctx)
	if err != nil {
		return params.TracingConfigResult{Error: apiservererrors.ServerError(err)}
	}

	result := params.TracingConfigResult{
		HTTPEndpoint:          config.HTTPEndpoint,
		GRPCEndpoint:          config.GRPCEndpoint,
		InsecureSkipVerify:    config.InsecureSkipVerify,
		StackTraces:           config.OpenTelemetryStackTraces,
		SampleRatio:           config.OpenTelemetrySampleRatio,
		TailSamplingThreshold: config.OpenTelemetryTailSamplingThreshold,
	}
	if config.CACertificate != "" {
		result.CACert = &config.CACertificate
	}
	return result
}

// WatchControllerTracingConfig starts a watcher to track changes to the
// controller-wide tracing configuration for the agent specified.
func (api *TracerAPI) WatchControllerTracingConfig(ctx context.Context, arg params.Entity) params.NotifyWatchResult {
	tag, err := names.ParseTag(arg.Tag)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}
	if !api.authorizer.AuthOwner(tag) {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(apiservererrors.ErrPerm)}
	}

	watch, err := api.controllerTracingConfigService.WatchWorkloadTracingConfig(ctx)
	if err != nil {
		return params.NotifyWatchResult{Error: apiservererrors.ServerError(err)}
	}

	result := params.NotifyWatchResult{}
	result.NotifyWatcherId, _, err = internal.EnsureRegisterWatcher(ctx, api.watcherRegistry, watch)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
	}
	return result
}
