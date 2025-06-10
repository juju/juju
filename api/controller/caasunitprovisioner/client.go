// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitprovisioner

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client allows access to the CAAS unit provisioner API endpoint.
type Client struct {
	facade base.FacadeCaller
}

// NewClient returns a client used to access the CAAS unit provisioner API.
func NewClient(caller base.APICaller, options ...Option) *Client {
	return &Client{
		facade: base.NewFacadeCaller(caller, "CAASUnitProvisioner", options...),
	}
}

func applicationTag(application string) (names.ApplicationTag, error) {
	if !names.IsValidApplication(application) {
		return names.ApplicationTag{}, errors.NotValidf("application name %q", application)
	}
	return names.NewApplicationTag(application), nil
}

func entities(tags ...names.Tag) params.Entities {
	entities := params.Entities{
		Entities: make([]params.Entity, len(tags)),
	}
	for i, tag := range tags {
		entities.Entities[i].Tag = tag.String()
	}
	return entities
}

// WatchApplications returns a StringsWatcher that notifies of
// changes to the lifecycles of CAAS applications in the current model.
func (c *Client) WatchApplications(ctx context.Context) (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	if err := c.facade.FacadeCall(ctx, "WatchApplications", nil, &result); err != nil {
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchApplication returns a NotifyWatcher that notifies of
// changes to the application in the current model.
func (c *Client) WatchApplication(ctx context.Context, appName string) (watcher.NotifyWatcher, error) {
	appTag, err := applicationTag(appName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return common.Watch(ctx, c.facade, "Watch", appTag)
}

// WatchApplicationScale returns a NotifyWatcher that notifies of
// changes to the lifecycles of units of the specified
// CAAS application in the current model.
func (c *Client) WatchApplicationScale(ctx context.Context, application string) (watcher.NotifyWatcher, error) {
	applicationTag, err := applicationTag(application)
	if err != nil {
		return nil, errors.Trace(err)
	}
	args := entities(applicationTag)

	var results params.NotifyWatchResults
	if err := c.facade.FacadeCall(ctx, "WatchApplicationsScale", args, &results); err != nil {
		return nil, err
	}
	if n := len(results.Results); n != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", n)
	}
	if err := results.Results[0].Error; err != nil {
		return nil, params.TranslateWellKnownError(err)
	}
	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), results.Results[0])
	return w, nil
}

// ApplicationScale returns the scale for the specified application.
func (c *Client) ApplicationScale(ctx context.Context, applicationName string) (int, error) {
	var results params.IntResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewApplicationTag(applicationName).String()}},
	}
	err := c.facade.FacadeCall(ctx, "ApplicationsScale", args, &results)
	if err != nil {
		return 0, errors.Trace(err)
	}
	if len(results.Results) != len(args.Entities) {
		return 0, errors.Errorf("expected %d result(s), got %d", len(args.Entities), len(results.Results))
	}
	if err := results.Results[0].Error; err != nil {
		return 0, params.TranslateWellKnownError(err)
	}
	return results.Results[0].Result, nil
}

// UpdateApplicationService updates the state model to reflect the state of the application's
// service as reported by the cloud.
func (c *Client) UpdateApplicationService(ctx context.Context, arg params.UpdateApplicationServiceArg) error {
	var result params.ErrorResults
	args := params.UpdateApplicationServiceArgs{Args: []params.UpdateApplicationServiceArg{arg}}
	if err := c.facade.FacadeCall(ctx, "UpdateApplicationsService", args, &result); err != nil {
		return errors.Trace(err)
	}
	if len(result.Results) != len(args.Args) {
		return errors.Errorf("expected %d result(s), got %d", len(args.Args), len(result.Results))
	}
	if result.Results[0].Error == nil {
		return nil
	}
	return params.TranslateWellKnownError(result.Results[0].Error)
}
