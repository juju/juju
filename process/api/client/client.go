// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/process/api"
)

const processAPI = "Process"

// processClient provides methods for interacting with Juju's internal
// RPC API, relative to workload processes.
type processClient struct {
	facade base.FacadeCaller
}

// NewProcessClient builds a new workload process API client.
func NewProcessClient(caller base.APICaller) *processClient {
	return &processClient{facade: base.NewFacadeCaller(caller, processAPI)}
}

// ListProcesses calls the ListProcesses API server method.
func (c *processClient) ListProcesses(tag string, ids ...string) ([]api.ProcessInfo, error) {
	var results []api.ProcessInfo

	args := api.ListProcessesArgs{Tag: tag, IDs: ids}
	if err := c.facade.FacadeCall("ListProcesses", args, &results); err != nil {
		return nil, errors.Trace(err)
	}

	return results, nil
}
