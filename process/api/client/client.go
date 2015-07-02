// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/process/api"
)

const processAPI = "Process"

type facadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// ProcessClient provides methods for interacting with Juju's internal
// RPC API, relative to workload processes.
type ProcessClient interface {
	RegisterProcesses(tag string, processes []api.ProcessInfo) ([]api.ProcessResult, error)
	ListProcesses(tag string, ids ...string) ([]api.ListProcessResult, error)
	SetProcessesStatus(tag, status string, ids ...string) error
	UnregisterProcesses(tag string, ids ...string) error
}

type processClient struct {
	base.ClientFacade
	facadeCaller
}

// NewProcessClient builds a new workload process API client.
func NewProcessClient(facade base.ClientFacade, caller facadeCaller) ProcessClient {
	return &processClient{facade, caller}
}

// RegisterProcesses calls the RegisterProcesses API server method.
func (c *processClient) RegisterProcesses(tag string, processes []api.ProcessInfo) ([]api.ProcessResult, error) {
	var result api.ProcessResults

	procArgs := make([]api.RegisterProcessArg, len(processes))
	for i, procInfo := range processes {
		procArg := api.RegisterProcessArg{UnitTag: tag, ProcessInfo: procInfo}
		procArgs[i] = procArg
	}

	args := api.RegisterProcessesArgs{Processes: procArgs}
	if err := c.FacadeCall("RegisterProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return result.Results, nil
}

// ListProcesses calls the ListProcesses API server method.
func (c *processClient) ListProcesses(tag string, ids ...string) ([]api.ListProcessResult, error) {
	var result api.ListProcessesResults

	args := api.ListProcessesArgs{UnitTag: tag, IDs: ids}
	if err := c.FacadeCall("ListProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	return result.Results, nil
}

// SetProcessesStatus calls the SetProcessesStatus API server method.
func (c *processClient) SetProcessesStatus(tag, status string, ids ...string) error {
	statusArgs := make([]api.SetProcessStatusArg, len(ids))
	for i, id := range ids {
		procStatus := api.ProcStatus{Status: status}
		statusArgs[i] = api.SetProcessStatusArg{UnitTag: tag, ID: id, Status: procStatus}
	}

	args := api.SetProcessesStatusArgs{Args: statusArgs}
	return c.FacadeCall("SetProcessesStatus", &args, nil)
}

// UnregisterProcesses calls the UnregisterProcesses API server method.
func (c *processClient) UnregisterProcesses(tag string, ids ...string) error {
	args := api.UnregisterProcessesArgs{UnitTag: tag, IDs: ids}
	return c.FacadeCall("UnregisterProcesses", &args, nil)
}
