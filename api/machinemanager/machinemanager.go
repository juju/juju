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

// DestroyMachinesWithParams removes the given set of machines, the semantics of which
// is determined by the force and keep parameters.
// TODO(wallyworld) - for Juju 3.0, this should be the preferred api to use.
func (client *Client) DestroyMachinesWithParams(force, keep bool, machines ...string) ([]params.DestroyMachineResult, error) {
	if !keep {
		if force {
			return client.ForceDestroyMachines(machines...)
		}
		return client.DestroyMachines(machines...)
	}
	args := params.DestroyMachinesParams{
		Force:       force,
		Keep:        keep,
		MachineTags: make([]string, 0, len(machines)),
	}
	allResults := make([]params.DestroyMachineResult, len(machines))
	index := make([]int, 0, len(machines))
	for i, machineId := range machines {
		if !names.IsValidMachine(machineId) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("machine ID %q", machineId).Error(),
			}
			continue
		}
		index = append(index, i)
		args.MachineTags = append(args.MachineTags, names.NewMachineTag(machineId).String())
	}
	if len(args.MachineTags) > 0 {
		var result params.DestroyMachineResults
		if err := client.facade.FacadeCall("DestroyMachineWithParams", args, &result); err != nil {
			return nil, errors.Trace(err)
		}
		if n := len(result.Results); n != len(args.MachineTags) {
			return nil, errors.Errorf("expected %d result(s), got %d", len(args.MachineTags), n)
		}
		for i, result := range result.Results {
			allResults[index[i]] = result
		}
	}
	return allResults, nil
}

func (client *Client) destroyMachines(method string, machines []string) ([]params.DestroyMachineResult, error) {
	args := params.Entities{
		Entities: make([]params.Entity, 0, len(machines)),
	}
	allResults := make([]params.DestroyMachineResult, len(machines))
	index := make([]int, 0, len(machines))
	for i, machineId := range machines {
		if !names.IsValidMachine(machineId) {
			allResults[i].Error = &params.Error{
				Message: errors.NotValidf("machine ID %q", machineId).Error(),
			}
			continue
		}
		index = append(index, i)
		args.Entities = append(args.Entities, params.Entity{
			Tag: names.NewMachineTag(machineId).String(),
		})
	}
	if len(args.Entities) > 0 {
		var result params.DestroyMachineResults
		if err := client.facade.FacadeCall(method, args, &result); err != nil {
			return nil, errors.Trace(err)
		}
		if n := len(result.Results); n != len(args.Entities) {
			return nil, errors.Errorf("expected %d result(s), got %d", len(args.Entities), n)
		}
		for i, result := range result.Results {
			allResults[index[i]] = result
		}
	}
	return allResults, nil
}
