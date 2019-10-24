// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package spacenamer

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

const spaceNamerFacade = "SpaceNamer"

type Client struct {
	facade base.FacadeCaller
}

// NewState creates a new instance mutater facade using the input caller.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, spaceNamerFacade)
	return NewClientFromFacade(facadeCaller)
}

// NewStateFromFacade creates a new instance mutater facade using the input
// facade caller.
func NewClientFromFacade(facadeCaller base.FacadeCaller) *Client {
	return &Client{
		facade: facadeCaller,
	}
}

// WatchDefaultSpaceConfig returns a NotifyWatcher reporting changes to the
// model's DefaultSpace config.
func (c *Client) WatchDefaultSpaceConfig() (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	err := c.facade.FacadeCall("WatchDefaultSpaceConfig", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	return apiwatcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}

// SetDefaultSpaceName sets the name of the default space from the model config.
func (c *Client) SetDefaultSpaceName() error {
	var result params.ErrorResult
	err := c.facade.FacadeCall("SetDefaultSpaceName", nil, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}
