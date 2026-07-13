// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tracer

import (
	"context"

	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/watcher"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the tracer facade client.
type Client struct {
	facade base.FacadeCaller
}

// ControllerTracingConfig holds the controller-wide tracing configuration
// for an OpenTelemetry collector.
type ControllerTracingConfig struct {
	HTTPEndpoint          string
	GRPCEndpoint          string
	CACert                string
	InsecureSkipVerify    *bool
	StackTraces           *bool
	SampleRatio           *float64
	TailSamplingThreshold *string
}

// NewClient returns a version of the tracer client that provides
// functionality required by the trace config updater worker.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{facade: base.NewFacadeCaller(caller, "Tracer", options...)}
}

// GetControllerTracingConfig returns the controller-wide tracing
// configuration for the agent specified by agentTag.
func (c *Client) GetControllerTracingConfig(ctx context.Context, agentTag names.Tag) (ControllerTracingConfig, error) {
	var result params.TracingConfigResult
	args := params.Entity{Tag: agentTag.String()}
	err := c.facade.FacadeCall(ctx, "GetControllerTracingConfig", args, &result)
	if err != nil {
		return ControllerTracingConfig{}, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return ControllerTracingConfig{}, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	var caCert string
	if result.CACert != nil {
		caCert = *result.CACert
	}
	return ControllerTracingConfig{
		HTTPEndpoint:          result.HTTPEndpoint,
		GRPCEndpoint:          result.GRPCEndpoint,
		CACert:                caCert,
		InsecureSkipVerify:    result.InsecureSkipVerify,
		StackTraces:           result.StackTraces,
		SampleRatio:           result.SampleRatio,
		TailSamplingThreshold: result.TailSamplingThreshold,
	}, nil
}

// WatchControllerTracingConfig returns a notify watcher that looks for changes
// in the controller-wide tracing configuration for the agent specified by
// agentTag.
func (c *Client) WatchControllerTracingConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	args := params.Entity{Tag: agentTag.String()}
	err := c.facade.FacadeCall(ctx, "WatchControllerTracingConfig", args, &result)
	if err != nil {
		return nil, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return nil, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
