// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package undertaker

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	"github.com/juju/juju/api/common/cloudspec"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// NewWatcherFunc exists to let us test Watch properly.
type NewWatcherFunc func(base.APICaller, params.NotifyWatchResult) watcher.NotifyWatcher

// Client provides access to the undertaker API
type Client struct {
	*cloudspec.CloudSpecAPI
	*common.ModelConfigWatcher
	modelTag   names.ModelTag
	caller     base.FacadeCaller
	newWatcher NewWatcherFunc
}

// NewClient creates a new client for accessing the undertaker API.
func NewClient(caller base.APICaller, newWatcher NewWatcherFunc, options ...Option) (*Client, error) {
	modelTag, ok := caller.ModelTag()
	if !ok {
		return nil, errors.New("undertaker client is not appropriate for controller-only API")
	}
	facadeCaller := base.NewFacadeCaller(caller, "Undertaker", options...)
	return &Client{
		modelTag:           modelTag,
		caller:             facadeCaller,
		newWatcher:         newWatcher,
		CloudSpecAPI:       cloudspec.NewCloudSpecAPI(facadeCaller, modelTag),
		ModelConfigWatcher: common.NewModelConfigWatcher(facadeCaller),
	}, nil
}

// ModelInfo returns information on the model needed by the undertaker worker.
func (c *Client) ModelInfo(ctx context.Context) (params.UndertakerModelInfoResult, error) {
	result := params.UndertakerModelInfoResult{}
	err := c.entityFacadeCall(ctx, "ModelInfo", &result)
	return result, errors.Trace(err)
}

// ProcessDyingModel checks if a dying model has any machines or applications.
// If there are none, the model's life is changed from dying to dead.
func (c *Client) ProcessDyingModel(ctx context.Context) error {
	return c.entityFacadeCall(ctx, "ProcessDyingModel", nil)
}

// RemoveModel removes any records of this model from Juju.
func (c *Client) RemoveModel(ctx context.Context) error {
	return c.entityFacadeCall(ctx, "RemoveModel", nil)
}

func (c *Client) entityFacadeCall(ctx context.Context, name string, results interface{}) error {
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.modelTag.String()}},
	}
	return c.caller.FacadeCall(ctx, name, args, results)
}

// WatchModelResources starts a watcher for changes to the model's
// machines and applications.
func (c *Client) WatchModelResources(ctx context.Context) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	err := c.entityFacadeCall(ctx, "WatchModelResources", &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := c.newWatcher(c.caller.RawAPICaller(), result)
	return w, nil
}

// WatchModel starts a watcher for changes to the model.
func (c *Client) WatchModel(ctx context.Context) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	err := c.entityFacadeCall(ctx, "WatchModel", &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := c.newWatcher(c.caller.RawAPICaller(), result)
	return w, nil
}
