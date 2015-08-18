// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/process"
	"github.com/juju/juju/process/api"
)

var logger = loggo.GetLogger("juju.process.api.client")

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
func (c HookContextClient) RegisterProcesses(processes ...process.Info) ([]string, error) {
	procArgs := make([]api.Process, len(processes))
	for i, proc := range processes {
		procArgs[i] = api.Proc2api(proc)
	}

	var result api.ProcessResults

	args := api.RegisterProcessesArgs{Processes: procArgs}
	if err := c.FacadeCall("RegisterProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, errors.Errorf(result.Error.GoString())
	}

	ids := make([]string, len(result.Results))
	for i, r := range result.Results {
		if r.Error != nil {
			return nil, errors.Errorf(r.Error.GoString())
		}
		ids[i] = r.ID
	}
	return ids, nil
}

// ListProcesses calls the ListProcesses API server method.
func (c HookContextClient) ListProcesses(ids ...string) ([]process.Info, error) {
	var result api.ListProcessesResults

	args := api.ListProcessesArgs{IDs: ids}
	if err := c.FacadeCall("ListProcesses", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	var notFound []string
	procs := make([]process.Info, len(result.Results))
	for i, presult := range result.Results {
		if presult.NotFound {
			notFound = append(notFound, presult.ID)
			continue
		}
		if presult.Error != nil {
			return procs, errors.Errorf(presult.Error.GoString())
		}
		pp := api.API2Proc(presult.Info)
		procs[i] = pp
	}
	if len(notFound) > 0 {
		return procs, errors.NotFoundf("%v", notFound)
	}
	return procs, nil
}

// SetProcessesStatus calls the SetProcessesStatus API server method.
func (c HookContextClient) SetProcessesStatus(status process.Status, pluginStatus process.PluginStatus, ids ...string) error {
	statusArgs := make([]api.SetProcessStatusArg, len(ids))
	for i, id := range ids {
		statusArgs[i] = api.SetProcessStatusArg{
			ID:           id,
			Status:       api.Status2apiStatus(status),
			PluginStatus: api.PluginStatus2apiPluginStatus(pluginStatus),
		}
	}

	args := api.SetProcessesStatusArgs{Args: statusArgs}
	return c.FacadeCall("SetProcessesStatus", &args, nil)
}

// Untrack calls the Untrack API server method.
func (c HookContextClient) Untrack(ids []string) error {
	args := api.UntrackArgs{IDs: ids}
	return c.FacadeCall("Untrack", &args, nil)
}
