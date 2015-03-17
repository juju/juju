// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

const converterAPI = "Converter"

// Converter provides common client-side API
// functions to call into apiserver.Converter
type State struct {
	facade base.FacadeCaller
}

// NewAPIAddresser returns a new APIAddresser that makes API calls
// using caller and the specified facade name.
func NewState(caller base.APICaller) *State {
	return &State{base.NewFacadeCaller(caller, converterAPI)}
}

// WatchAPIHostPorts watches the host/port addresses of the API servers.
func (c *State) WatchForJobsChanges(tag string) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResult
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}
	err := c.facade.FacadeCall("WatchForJobsChanges", args, &result)
	if err != nil {
		return nil, err
	}
	return watcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}
