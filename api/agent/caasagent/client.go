// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasagent

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to an agent's view of state.
type Client struct {
	facade base.FacadeCaller
	*cloudspec.CloudSpecAPI
	*common.ModelConfigWatcher
	*common.ControllerConfigAPI

	modelTag names.ModelTag
}

// NewClient returns a version of an api client that provides functionality
// required by caas agent code.
func NewClient(caller base.APICaller, options ...Option) (*Client, error) {
	modelTag, isModel := caller.ModelTag()
	if !isModel {
		return nil, errors.New("expected model specific API connection")
	}
	facadeCaller := base.NewFacadeCaller(caller, "CAASAgent", options...)
	return &Client{
		facade:              facadeCaller,
		CloudSpecAPI:        cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
		ModelConfigWatcher:  common.NewModelConfigWatcher(facadeCaller),
		ControllerConfigAPI: common.NewControllerConfig(facadeCaller),
		modelTag:            modelTag,
	}, nil
}

// WatchCloudSpecChanges returns a NotifyWatcher waiting for the
// model's cloud to change.
func (c *Client) WatchCloudSpecChanges(ctx context.Context) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{Entities: []params.Entity{{Tag: c.modelTag.String()}}}
	err := c.facade.FacadeCall(ctx, "WatchCloudSpecsChanges", args, &results)
	if err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Annotate(result.Error, "API request failed")
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
