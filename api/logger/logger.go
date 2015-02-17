// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"

	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

// State provides access to an logger worker's view of the state.
type State struct {
	facade base.FacadeCaller
}

// NewState returns a version of the state that provides functionality
// required by the logger worker.
func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, "Logger")}
}

// LoggingConfig returns the loggo configuration string for the agent
// specified by agentTag.
func (st *State) LoggingConfig(agentTag names.Tag) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := st.facade.FacadeCall("LoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return "", err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return "", fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if err := result.Error; err != nil {
		return "", err
	}
	return result.Result, nil
}

// WatchLoggingConfig returns a notify watcher that looks for changes in the
// logging-config for the agent specified by agentTag.
func (st *State) WatchLoggingConfig(agentTag names.Tag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag.String()}},
	}
	err := st.facade.FacadeCall("WatchLoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.facade.RawAPICaller(), result)
	return w, nil
}
