// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

// TODO(ericsnow) Eliminate the apiserver/common import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

var logger = loggo.GetLogger("juju.process.api.server")

// UnitProcesses exposes the State functionality for a unit's
// workload processes.
type UnitProcesses interface {
	// Add registers a workload process for the unit and info.
	Add(info process.Info) error
	// List returns information on the process with the id on the unit.
	List(ids ...string) ([]process.Info, error)
	// ListDefinitions returns the process definitions found in the
	// unit's metadata.
	ListDefinitions() ([]charm.Process, error)
	// Settatus sets the status for the process with the given id on the unit.
	SetStatus(id string, status process.CombinedStatus) error
	// Remove removes the information for the process with the given id.
	Remove(id string) error
}

// HookContextAPI serves workload process-specific API methods.
type HookContextAPI struct {
	// State exposes the workload process aspect of Juju's state.
	State UnitProcesses
}

// NewHookContextAPI builds a new facade for the given State.
func NewHookContextAPI(st UnitProcesses) *HookContextAPI {
	return &HookContextAPI{State: st}
}

// ListDefinitions builds the list of workload process definitions
// found in the metadata of the unit's charm.
func (a HookContextAPI) ListDefinitions() (api.ListDefinitionsResults, error) {
	var results api.ListDefinitionsResults

	definitions, err := a.State.ListDefinitions()
	if err != nil {
		results.Error = common.ServerError(err)
		return results, nil
	}

	for _, definition := range definitions {
		result := api.Definition2api(definition)
		results.Results = append(results.Results, result)
	}
	return results, nil
}

// RegisterProcess registers a workload process in state.
func (a HookContextAPI) RegisterProcesses(args api.RegisterProcessesArgs) (api.ProcessResults, error) {
	logger.Tracef("registering %d procs from API", len(args.Processes))

	r := api.ProcessResults{}
	for _, apiProc := range args.Processes {
		info := api.API2Proc(apiProc)
		logger.Tracef("registering proc from API: %#v", info)
		res := api.ProcessResult{
			ID: info.ID(),
		}
		if err := a.State.Add(info); err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}

		r.Results = append(r.Results, res)
	}
	return r, nil
}

// ListProcesses builds the list of workload processes registered for
// the given unit and IDs. If no IDs are provided then all registered
// processes for the unit are returned.
func (a HookContextAPI) ListProcesses(args api.ListProcessesArgs) (api.ListProcessesResults, error) {
	var r api.ListProcessesResults

	ids := args.IDs
	procs, err := a.State.List(ids...)
	if err != nil {
		r.Error = common.ServerError(err)
		return r, nil
	}

	if len(ids) == 0 {
		for _, proc := range procs {
			ids = append(ids, proc.ID())
		}
	}

	for _, id := range ids {
		res := api.ListProcessResult{
			ID: id,
		}

		found := false
		for _, proc := range procs {
			procID := proc.Name
			if proc.Details.ID != "" {
				procID += "/" + proc.Details.ID
			}
			if id == proc.ID() {
				res.Info = api.Proc2api(proc)
				found = true
				break
			}
		}
		if !found {
			res.NotFound = true
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// SetProcessesStatus sets the raw status of a workload process.
func (a HookContextAPI) SetProcessesStatus(args api.SetProcessesStatusArgs) (api.ProcessResults, error) {
	r := api.ProcessResults{}
	for _, arg := range args.Args {
		res := api.ProcessResult{
			ID: arg.ID,
		}
		err := a.State.SetStatus(arg.ID, process.CombinedStatus{
			Status:       api.APIStatus2Status(arg.Status),
			PluginStatus: api.APIPluginStatus2PluginStatus(arg.PluginStatus),
		})
		if err != nil {
			res.Error = common.ServerError(err)
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}

// UnregisterProcesses marks the identified process as unregistered.
func (a HookContextAPI) UnregisterProcesses(args api.UnregisterProcessesArgs) (api.ProcessResults, error) {
	r := api.ProcessResults{}
	for _, id := range args.IDs {
		res := api.ProcessResult{
			ID: id,
		}
		if err := a.State.Remove(id); err != nil {
			res.Error = common.ServerError(errors.Trace(err))
			r.Error = common.ServerError(api.BulkFailure)
		}
		r.Results = append(r.Results, res)
	}
	return r, nil
}
