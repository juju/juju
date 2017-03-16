// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

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

// DestroyMachines removes a given set of machines.
func (client *Client) DestroyMachines(machines ...string) ([]params.DestroyMachineResult, error) {
	return client.destroyMachines("DestroyMachine", machines)
}

// ForceDestroyMachines removes a given set of machines and all
// associated units.
func (client *Client) ForceDestroyMachines(machines ...string) ([]params.DestroyMachineResult, error) {
	return client.destroyMachines("ForceDestroyMachine", machines)
}

func (client *Client) destroyMachines(method string, machines []string) ([]params.DestroyMachineResult, error) {
	entities := params.Entities{
		Entities: make([]params.Entity, len(machines)),
	}
	for i, machineId := range machines {
		if !names.IsValidMachine(machineId) {
			return nil, errors.NotValidf("machine ID %q", machineId)
		}
		entities.Entities[i].Tag = names.NewMachineTag(machineId).String()
	}
	var out params.DestroyMachineResults
	if err := client.facade.FacadeCall(method, entities, &out); err != nil {
		return nil, errors.Trace(err)
	}
	if n := len(out.Results); n != len(machines) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(machines), n)
	}
	return out.Results, nil
}
