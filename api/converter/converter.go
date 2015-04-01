// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// package converter exposes a watcher for machines to tell when they've been
// updated, and an api endpoint to get the machine's jobs.  This is used by
// ensure- availability to convert existing machines in the environment into
// state servers.
package converter

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.converter")

const converterAPI = "Converter"

// State exposes a machine watcher and an API endpoint to get a machine's jobs.
type State struct {
	facade base.FacadeCaller
}

// NewState returns a new state object wrapping the given caller.
func NewState(caller base.APICaller) *State {
	return &State{facade: base.NewFacadeCaller(caller, converterAPI)}
}

// WatchMachine returns a watcher that watches the machine with the given
// tag.
func (c *State) WatchMachine(tag names.MachineTag) (watcher.NotifyWatcher, error) {
	var results params.NotifyWatchResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}
	err := c.facade.FacadeCall("WatchMachines", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return watcher.NewNotifyWatcher(c.facade.RawAPICaller(), result), nil
}

// Jobs returns a list of jobs for the machine with the given tag.
func (c *State) Jobs(tag names.MachineTag) (*params.JobsResult, error) {
	var results params.JobsResults
	args := params.Entities{
		Entities: []params.Entity{{Tag: tag.String()}},
	}

	err := c.facade.FacadeCall("Jobs", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	return &result, nil
}
