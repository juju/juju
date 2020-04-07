// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgradesteps

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const upgradeStepsFacade = "UpgradeSteps"

type Client struct {
	facade base.FacadeCaller
}

// NewState creates a new upgrade steps facade using the input caller.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, upgradeStepsFacade)
	return NewClientFromFacade(facadeCaller)
}

// NewStateFromFacade creates a new upgrade steps facade using the input
// facade caller.
func NewClientFromFacade(facadeCaller base.FacadeCaller) *Client {
	return &Client{
		facade: facadeCaller,
	}
}

// ResetKVMMachineModificationStatusIdle
func (c *Client) ResetKVMMachineModificationStatusIdle(tag names.Tag) error {
	var result params.ErrorResult
	arg := params.Entity{tag.String()}
	err := c.facade.FacadeCall("ResetKVMMachineModificationStatusIdle", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// WriteAgentState writes the given internal agent state for the provided
// units. Currently this call only handles the uniter's state.
func (c *Client) WriteAgentState(args []params.SetUnitStateArg) error {
	var result params.ErrorResults
	arg := params.SetUnitStateArgs{Args: args}
	err := c.facade.FacadeCall("WriteAgentState", arg, &result)
	if err != nil {
		return errors.Trace(err)
	}
	return result.Combine()
}
