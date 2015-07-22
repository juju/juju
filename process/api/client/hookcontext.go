// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

const processAPI = "Process"

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// HookContextClient provides methods for interacting with Juju's internal
// RPC API, relative to workload processes.
type HookContextClient struct {
	facadeCaller
}

// NewHookContextClient builds a new workload process API client.
func NewHookContextClient(caller facadeCaller) HookContextClient {
	return HookContextClient{caller}
}

// AllDefinitions calls the ListDefinitions API server method.
func (c HookContextClient) AllDefinitions() ([]charm.Process, error) {
	var results api.ListDefinitionsResults
	if err := c.FacadeCall("ListDefinitions", nil, &results); err != nil {
		return nil, errors.Trace(err)
	}
	if results.Error != nil {
		return nil, errors.Errorf(results.Error.GoString())
	}

	var definitions []charm.Process
	for _, result := range results.Results {
		definition := api.API2Definition(result)
		definitions = append(definitions, definition)
	}
	return definitions, nil
}

// RegisterProcesses calls the RegisterProcesses API server method.
func (c HookContextClient) RegisterProcesses(processes ...api.Process) ([]api.ProcessResult, error) {
	var result api.ProcessResults

	args := api.RegisterProcessesArgs{Processes: processes}
	if err := c.FacadeCall("RegisterProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Errorf(result.Error.GoString())
	}

	return result.Results, nil
}

// ListProcesses calls the ListProcesses API server method.
func (c HookContextClient) ListProcesses(ids ...string) ([]api.ListProcessResult, error) {
	var result api.ListProcessesResults

	args := api.ListProcessesArgs{IDs: ids}
	if err := c.FacadeCall("ListProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return result.Results, nil
}

// SetProcessesStatus calls the SetProcessesStatus API server method.
func (c HookContextClient) SetProcessesStatus(status string, ids ...string) error {
	statusArgs := make([]api.SetProcessStatusArg, len(ids))
	for i, id := range ids {
		procStatus := api.ProcessStatus{Label: status}
		statusArgs[i] = api.SetProcessStatusArg{ID: id, Status: procStatus}
	}

	args := api.SetProcessesStatusArgs{Args: statusArgs}
	return c.FacadeCall("SetProcessesStatus", &args, nil)
}

// UnregisterProcesses calls the UnregisterProcesses API server method.
func (c HookContextClient) UnregisterProcesses(ids ...string) error {
	args := api.UnregisterProcessesArgs{IDs: ids}
	return c.FacadeCall("UnregisterProcesses", &args, nil)
}

// Context Method Implementations
func (c HookContextClient) List() ([]string, error) {
	results, err := c.ListProcesses()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := make([]string, len(results))
	for i, proc := range results {
		ids[i] = proc.ID
	}
	return ids, nil
}

func (c HookContextClient) Get(ids ...string) ([]*process.Info, error) {
	results, err := c.ListProcesses(ids...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var notFound []string
	procs := make([]*process.Info, len(results))
	for i, presult := range results {
		if presult.NotFound {
			notFound = append(notFound, presult.ID)
			continue
		}
		pp := api.API2Proc(presult.Info)
		procs[i] = &pp
	}
	if len(notFound) > 0 {
		return procs, errors.NotFoundf("%v", notFound)
	}
	return procs, nil
}

func (c HookContextClient) Set(procs ...*process.Info) error {
	procArgs := make([]api.Process, len(procs))
	for i, proc := range procs {
		procArgs[i] = api.Proc2api(*proc)
	}
	_, err := c.RegisterProcesses(procArgs...)
	return err
}
