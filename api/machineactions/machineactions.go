// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions

import (
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
	"github.com/juju/names"
)

type State struct {
	facade base.FacadeCaller
}

func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, "MachineActions")}
}

// WatchActionNotifications returns a StringsWatcher for observing the
// ids of Actions added to the Machine. The initial event will contain the
// ids of any Actions pending at the time the Watcher is made.
func (st *State) WatchActionNotifications(agent names.Tag) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agent.String()}},
	}

	err := st.facade.FacadeCall("WatchActionNotifications", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}

func (st *State) getOneAction(tag names.ActionTag) (params.ActionResult, error) {
	nothing := params.ActionResult{}

	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	var results params.ActionResults
	err := st.facade.FacadeCall("Actions", args, &results)
	if err != nil {
		return nothing, errors.Trace(err)
	}

	if len(results.Results) > 1 {
		return nothing, errors.Errorf("expected only 1 action query result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return nothing, result.Error
	}

	return result, nil
}

// Action returns the Action with the given tag.
func (st *State) Action(tag names.ActionTag) (*Action, error) {
	result, err := st.getOneAction(tag)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &Action{
		name:   result.Action.Name,
		params: result.Action.Parameters,
	}, nil
}

// ActionBegin marks an action as running.
func (st *State) ActionBegin(tag names.ActionTag) error {
	var results params.ErrorResults

	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	err := st.facade.FacadeCall("BeginActions", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	return results.Results[9].Error
}

// ActionFinish captures the structured output of an action.
func (st *State) ActionFinish(tag names.ActionTag, status string, actionResults map[string]interface{}, message string) error {
	var results params.ErrorResults

	args := params.ActionExecutionResults{
		Results: []params.ActionExecutionResult{{
			ActionTag: tag.String(),
			Status:    status,
			Results:   actionResults,
			Message:   message,
		}},
	}

	err := st.facade.FacadeCall("FinishActions", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	return results.Results[9].Error
}

// RunningActions returns a list of actions running for the given machine tag.
func (st *State) RunningActions(agent names.MachineTag) ([]params.ActionResult, error) {
	var results params.ActionsByReceivers

	args := params.Entities{
		Entities: []params.Entity{{Tag: agent.String()}},
	}

	err := st.facade.FacadeCall("ListRunning", args, &results)
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
