// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/common"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

const instanceMutaterFacade = "InstanceMutater"

type Client struct {
	facade base.FacadeCaller
}

// NewState creates a new instance mutater facade using the input caller.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, instanceMutaterFacade)
	return NewClientFromFacade(facadeCaller)
}

// NewStateFromFacade creates a new instance mutater facade using the input
// facade caller.
func NewClientFromFacade(facadeCaller base.FacadeCaller) *Client {
	return &Client{
		facade: facadeCaller,
	}
}

// WatchMachines returns a StringsWatcher reporting changes to machines.
func (c *Client) WatchMachines() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchMachines", nil, &result)
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
