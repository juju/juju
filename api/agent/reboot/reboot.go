// Copyright 2014 Cloudbase Solutions
// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package reboot

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
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

// Client provides access to an reboot worker's client facade.
type Client interface {
	// WatchForRebootEvent returns a watcher.NotifyWatcher that
	// reacts to reboot flag changes.
	WatchForRebootEvent() (watcher.NotifyWatcher, error)

	// RequestReboot sets the reboot flag for the calling machine.
	RequestReboot() error

	// ClearReboot clears the reboot flag for the calling machine.
	ClearReboot() error

	// GetRebootAction returns the reboot action for the calling machine.
	GetRebootAction() (params.RebootAction, error)
}

var _ Client = (*client)(nil)

// client implements Client.
type client struct {
	machineTag names.Tag
	facade     base.FacadeCaller
}

// NewClient returns a version of the client that provides functionality
// required by the reboot worker.
func NewClient(caller base.APICaller, machineTag names.MachineTag, options ...Option) Client {
	return &client{
		facade:     base.NewFacadeCaller(caller, "Reboot", options...),
		machineTag: machineTag,
	}
}

// NewFromConnection returns access to the Reboot API
func NewFromConnection(c api.Connection) (Client, error) {
	switch tag := c.AuthTag().(type) {
	case names.MachineTag:
		return NewClient(c, tag), nil
	default:
		return nil, errors.Errorf("expected names.MachineTag, got %T", tag)
	}
}

// WatchForRebootEvent implements Client.WatchForRebootEvent
func (c *client) WatchForRebootEvent() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult

	if err := c.facade.FacadeCall(context.TODO(), "WatchForRebootEvent", nil, &result); err != nil {
		return nil, err
	}
	if result.Error != nil {
		return nil, result.Error
	}

	w := apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// RequestReboot implements Client.RequestReboot
func (c *client) RequestReboot() error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.machineTag.String()}},
	}

	err := c.facade.FacadeCall(context.TODO(), "RequestReboot", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return errors.Trace(results.Results[0].Error)
	}
	return nil
}

// ClearReboot implements Client.ClearReboot
func (c *client) ClearReboot() error {
	var results params.ErrorResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.machineTag.String()}},
	}

	err := c.facade.FacadeCall(context.TODO(), "ClearReboot", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return errors.Trace(results.Results[0].Error)
	}

	return nil
}

// GetRebootAction implements Client.GetRebootAction
func (c *client) GetRebootAction() (params.RebootAction, error) {
	var results params.RebootActionResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: c.machineTag.String()}},
	}

	err := c.facade.FacadeCall(context.TODO(), "GetRebootAction", args, &results)
	if err != nil {
		return params.ShouldDoNothing, err
	}
	if len(results.Results) != 1 {
		return params.ShouldDoNothing, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	if results.Results[0].Error != nil {
		return params.ShouldDoNothing, errors.Trace(results.Results[0].Error)
	}

	return results.Results[0].Result, nil
}
