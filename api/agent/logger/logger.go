// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

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

// Client provides access to a logger facade client.
type Client struct {
	facade base.FacadeCaller
}

// ControllerLokiConfig holds the controller-wide Loki push API configuration.
type ControllerLokiConfig struct {
	Endpoint           string
	CACert             string
	InsecureSkipVerify *bool
}

// NewClient returns a version of the logger client that provides functionality
// required by the logger worker.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{facade: base.NewFacadeCaller(caller, "Logger", options...)}
}

// LoggingConfig returns the loggo configuration string for the agent
// specified by agentTag.
func (c *Client) LoggingConfig(ctx context.Context, agentTag names.Tag) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall(ctx, "LoggingConfig", args, &results)
	if err != nil {
		return "", internalerrors.Capture(err)
	}
	if len(results.Results) != 1 {
		return "", internalerrors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return "", internalerrors.Capture(err)
	}
	return result.Result, nil
}

// GetControllerLokiConfig returns the controller-wide Loki configuration for
// the agent specified by agentTag.
func (c *Client) GetControllerLokiConfig(ctx context.Context, agentTag names.Tag) (ControllerLokiConfig, error) {
	var result params.LokiConfigResult
	args := params.Entity{Tag: agentTag.String()}
	err := c.facade.FacadeCall(ctx, "GetControllerLokiConfig", args, &result)
	if err != nil {
		return ControllerLokiConfig{}, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return ControllerLokiConfig{}, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	var caCert string
	if result.CACert != nil {
		caCert = *result.CACert
	}
	return ControllerLokiConfig{
		Endpoint:           result.Endpoint,
		CACert:             caCert,
		InsecureSkipVerify: result.InsecureSkipVerify,
	}, nil
}

// WatchLoggingConfig returns a notify watcher that looks for changes in the
// logging-config for the agent specified by agentTag.
func (c *Client) WatchLoggingConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall(ctx, "WatchLoggingConfig", args, &results)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	if len(results.Results) != 1 {
		return nil, internalerrors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, internalerrors.Capture(result.Error)
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}

// WatchControllerLokiConfig returns a notify watcher that looks for changes in
// the controller-wide Loki configuration for the agent specified by agentTag.
func (c *Client) WatchControllerLokiConfig(ctx context.Context, agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	args := params.Entity{Tag: agentTag.String()}
	err := c.facade.FacadeCall(ctx, "WatchControllerLokiConfig", args, &result)
	if err != nil {
		return nil, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	if err := result.Error; err != nil {
		return nil, internalerrors.Capture(apiservererrors.RestoreError(err))
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
