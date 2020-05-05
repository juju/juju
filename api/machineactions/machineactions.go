// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

// machineactions implements the the api side of
// running actions on machines
package machineactions

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/watcher"
)

type Client struct {
	facade base.FacadeCaller
}

func NewClient(caller base.APICaller) *Client {
	return &Client{base.NewFacadeCaller(caller, "MachineActions")}
}

// WatchActionNotifications returns a StringsWatcher for observing the
// IDs of Actions added to the Machine. The initial event will contain the
// IDs of any Actions pending at the time the Watcher is made.
func (c *Client) WatchActionNotifications(agent names.MachineTag) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agent.String()}},
	}

	err := c.facade.FacadeCall("WatchActionNotifications", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return nil, errors.Trace(result.Error)
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

func (c *Client) getOneAction(tag names.ActionTag) (params.ActionResult, error) {
	nothing := params.ActionResult{}

	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	var results params.ActionResults
	err := c.facade.FacadeCall("Actions", args, &results)
	if err != nil {
		return nothing, errors.Trace(err)
	}

	if len(results.Results) > 1 {
		return nothing, errors.Errorf("expected only 1 action query result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return nothing, errors.Trace(result.Error)
	}

	return result, nil
}

// Action returns the Action with the given tag.
func (c *Client) Action(tag names.ActionTag) (*Action, error) {
	result, err := c.getOneAction(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Action{
		name:   result.Action.Name,
		params: result.Action.Parameters,
	}, nil
}

// ActionBegin marks an action as running.
func (c *Client) ActionBegin(tag names.ActionTag) error {
	var results params.ErrorResults

	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	err := c.facade.FacadeCall("BeginActions", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	return results.OneError()
}

// ActionFinish captures the structured output of an action.
func (c *Client) ActionFinish(tag names.ActionTag, status string, actionResults map[string]interface{}, message string) error {
	var results params.ErrorResults

	args := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: tag.String(),
			Status:    status,
			Results:   actionResults,
			Message:   message,
		}},
	}

	err := c.facade.FacadeCall("FinishActions", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	return results.OneError()
}

// RunningActions returns a list of actions running for the given machine tag.
func (c *Client) RunningActions(agent names.MachineTag) ([]params.ActionResult, error) {
	var results params.ActionsByReceivers

	args := params.Entities{
		Entities: []params.Entity{{Tag: agent.String()}},
	}

	err := c.facade.FacadeCall("RunningActions", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(results.Actions) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Actions))
	}

	result := results.Actions[0]
	if result.Error != nil {
		return nil, result.Error
	}

	return result.Actions, nil
}
