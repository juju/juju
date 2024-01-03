// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

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

const instanceMutaterFacade = "InstanceMutater"

type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new instance mutater facade using the input caller.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, instanceMutaterFacade, options...)
	return NewClientFromFacade(facadeCaller)
}

// NewClientFromFacade creates a new instance mutater facade using the input
// facade caller.
func NewClientFromFacade(facadeCaller base.FacadeCaller) *Client {
	return &Client{
		facade: facadeCaller,
	}
}

// WatchModelMachines returns a StringsWatcher reporting changes to machines
// and not containers.
func (c *Client) WatchModelMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall(context.TODO(), "WatchModelMachines", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result), nil
}

// Machine provides access to methods of a state.Machine through the
// facade.
func (c *Client) Machine(tag names.MachineTag) (MutaterMachine, error) {
	life, err := common.OneLife(c.facade, tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Machine{c.facade, tag, life}, nil
}
