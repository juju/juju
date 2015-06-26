// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const machineManagerFacade = "MachineManager"

// Client provides access to the machinemanager, used to add machines to state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new machinemanager client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, machineManagerFacade)
	return &Client{ClientFacade: frontend, facade: backend}
}

// AddMachines adds new machines with the supplied parameters, creating any requested disks.
func (client *Client) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)
	err := client.facade.FacadeCall("AddMachines", args, results)
	if len(results.Machines) != len(machineParams) {
		return nil, errors.Errorf("expected %d result, got %d", len(machineParams), len(results.Machines))
	}
	return results.Machines, err
}
