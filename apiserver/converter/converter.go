// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package converter

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/multiwatcher"
	"github.com/juju/juju/state/watcher"
)

var logger = loggo.GetLogger("juju.apiserver.converter")

func init() {
	common.RegisterStandardFacade("Converter", 1, NewConverterAPI)
}

// ConverterAPI contains methods for use with ensure-availability, watching
// machines and their jobs.
type ConverterAPI struct {
	st         *state.State
	resources  *common.Resources
	authorizer common.Authorizer
}

// NewConverterAPI returns a new instance of the API.
func NewConverterAPI(
	st *state.State,
	resources *common.Resources,
	authorizer common.Authorizer,
) (*ConverterAPI, error) {
	if !authorizer.AuthMachineAgent() {
		return nil, common.ErrPerm
	}

	return &ConverterAPI{
		st:         st,
		resources:  resources,
		authorizer: authorizer,
	}, nil
}

func (c *ConverterAPI) getMachine(tag names.Tag) (*state.Machine, error) {
	if tag.Kind() != names.MachineTagKind {
		return nil, common.ErrPerm
	}

	entity, err := c.st.FindEntity(tag)
	if err != nil {
		return nil, err
	}
	return entity.(*state.Machine), nil
}

// Jobs returns the jobs assigned to the given entities.
func (c *ConverterAPI) Jobs(args params.Entities) (params.JobsResults, error) {
	result := params.JobsResults{
		Results: make([]params.JobsResult, len(args.Entities)),
	}

	for i, agent := range args.Entities {
		tag, err := names.ParseMachineTag(agent.Tag)
		if err != nil {
			logger.Warningf("not a machine tag: %v", agent.Tag)
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}

		err = common.ErrPerm
		if c.authorizer.AuthOwner(tag) {
			machine, err := c.getMachine(tag)
			if err != nil {
				logger.Warningf("can't get machine for tag %q: %v", tag, err)
				result.Results[i].Error = common.ServerError(err)
				continue
			}
			machineJobs := machine.Jobs()
			jobs := make([]multiwatcher.MachineJob, len(machineJobs))
			for i, job := range machineJobs {
				jobs[i] = job.ToParams()
			}
			result.Results[i].Jobs = jobs
		}
	}
	return result, nil
}

// WatchMachines creates a watcher for each of the specified machines.
func (c *ConverterAPI) WatchMachines(args params.Entities) (params.NotifyWatchResults, error) {
	result := params.NotifyWatchResults{
		Results: make([]params.NotifyWatchResult, len(args.Entities)),
	}
	for i, ent := range args.Entities {
		tag, err := names.ParseMachineTag(ent.Tag)
		if err != nil {
			logger.Warningf("not a machine tag: %v", ent.Tag)
			result.Results[i].Error = common.ServerError(common.ErrPerm)
			continue
		}
		if c.authorizer.AuthOwner(tag) {
			machine, err := c.getMachine(tag)
			if err != nil {
				logger.Warningf("can't get machine for tag %q: %v", tag, err)
				result.Results[i].Error = common.ServerError(err)
				continue
			}

			watch := machine.Watch()
			// Consume the initial event. Technically, API
			// calls to Watch 'transmit' the initial event
			// in the Watch response. But NotifyWatchers
			// have no state to transmit.
			if _, ok := <-watch.Changes(); ok {
				result.Results[i].NotifyWatcherId = c.resources.Register(watch)
			} else {
				err := watcher.EnsureErr(watch)
				result.Results[i].Error = common.ServerError(err)
			}
		}
	}
	return result, nil
}
