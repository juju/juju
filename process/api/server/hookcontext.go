// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

var logger = loggo.GetLogger("juju.process.api.server")

// HookContextAPI serves workload process-specific API methods.
type HookContextAPI struct {
	State State
}

// State is an interface that exposes functionality this package needs to wrap
// in an API.
type State interface {
	// RegisterProcess registers a workload process for the given unit and info.
	RegisterProcess(unit names.UnitTag, info process.Info) error
	// ListProcess returns information on the process with the id on the unit.
	ListProcess(unit names.UnitTag, id string) (process.Info, error)
	// SetProcessStatus sets the status for the process with the given id on the unit.
	SetProcessStatus(unit names.UnitTag, id string, status string) error
	// UnregisterProcess removes the information for the process with the given id.
	UnregisterProcess(unit names.UnitTag, id string) error
}

// RegisterProcess registers a workload process in state.
func (a HookContextAPI) RegisterProcesses(args api.RegisterProcessesArgs) (api.ProcessResults, error) {
	r := api.ProcessResults{}
	for _, arg := range args.Processes {
		info := api.API2Proc(arg.ProcessInfo)
		unit := names.NewUnitTag(arg.UnitTag)

		res := api.ProcessResult{ID: arg.Details.ID}

		if err := errors.Trace(a.State.RegisterProcess(unit, info)); err != nil {
			res.Error = common.ServerError(err)
		}

		r.Results = append(r.Results, res)
	}
	return r, nil
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (a HookContextAPI) ListProcesses(args api.ListProcessesArgs) (api.ListProcessesResults, error) {
	r := api.ListProcessesResults{}
	unit := names.NewUnitTag(args.UnitTag)
	for _, id := range args.IDs {
		info, err := a.State.ListProcess(unit, id)
		res := api.ListProcessResult{
			ID: id,
		}
		if err != nil {
			res.Error = common.ServerError(err)
		} else {
			res.Info = api.Proc2api(info)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetProcessesStatus sets the raw status of a workload process.
func (a HookContextAPI) SetProcessesStatus(args api.SetProcessesStatusArgs) (api.ProcessResults, error) {
	r := api.ProcessResults{}
	for _, arg := range args.Args {
		res := api.ProcessResult{ID: arg.ID}

		unit := names.NewUnitTag(arg.UnitTag)
		err := a.State.SetProcessStatus(unit, arg.ID, arg.Status.Status)
		if err != nil {
			res.Error = common.ServerError(err)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// UnregisterProcesses marks the identified process as unregistered.
func (a HookContextAPI) UnregisterProcesses(args api.UnregisterProcessesArgs) (api.ProcessResults, error) {
	r := api.ProcessResults{}
	unit := names.NewUnitTag(args.UnitTag)
	for _, id := range args.IDs {
		res := api.ProcessResult{ID: id}
		err := errors.Trace(a.State.UnregisterProcess(unit, id))
		if err != nil {
			res.Error = common.ServerError(err)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}
