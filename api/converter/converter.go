// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.apiserver.converter")

const converterAPI = "Converter"

// Converter provides common client-side API
// functions to call into apiserver.Converter
type State struct {
	facade base.FacadeCaller
}

func NewState(caller base.APICaller) *State {
	return &State{facade: base.NewFacadeCaller(caller, converterAPI)}
}

func (c *State) WatchForJobsChanges(tag string) (watcher.NotifyWatcher, error) {
	var result params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag}},
	}

	logger.Infof("calling facade WatchForJobsChanges")
	err := c.facade.FacadeCall("WatchForJobsChanges", args, &result)
	if err != nil {
		return nil, err
	}

	return watcher.NewNotifyWatcher(c.facade.RawAPICaller(), result.Results[0]), nil
}
