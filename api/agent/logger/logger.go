// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"context"
	"fmt"

	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
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

// NewClient returns a version of the logger client that provides functionality
// required by the logger worker.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{base.NewFacadeCaller(caller, "Logger", options...)}
}

// LoggingConfig returns the loggo configuration string for the agent
// specified by agentTag.
func (c *Client) LoggingConfig(agentTag names.Tag) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall(context.TODO(), "LoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return "", err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// WatchLoggingConfig returns a notify watcher that looks for changes in the
// logging-config for the agent specified by agentTag.
func (c *Client) WatchLoggingConfig(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := c.facade.FacadeCall(context.TODO(), "WatchLoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
