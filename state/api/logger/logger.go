// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"fmt"

	"launchpad.net/juju-core/state/api/common"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/watcher"
)

// State provides access to an logger worker's view of the state.
type State struct {
	caller common.Caller
}

// NewState returns a version of the state that provides functionality
// required by the logger worker.
func NewState(caller common.Caller) *State {
	return &State{caller}
}

func (st *State) LoggingConfig(tag string) (string, error) {
	var results params.StringResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := st.caller.Call("Logger", "", "LoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if err := result.Error; err != nil {
		return nil, err
	}
	return result.Result, nil
}

func (st *State) WatchLoggingConfig(agentTag string) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: agentTag}},
	}
	err := st.caller.Call("Logger", "", "WatchLoggingConfig", args, &results)
	if err != nil {
		// TODO: Not directly tested
		return nil, err
	}
	if len(results.Results) != 1 {
		// TODO: Not directly tested
		return nil, fmt.Errorf("expected one result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		//  TODO: Not directly tested
		return nil, result.Error
	}
	w := watcher.NewNotifyWatcher(st.caller, result)
	return w, nil
}
