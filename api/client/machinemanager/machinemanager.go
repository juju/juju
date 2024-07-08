// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"context"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const machineManagerFacade = "MachineManager"

// Client provides access to the machinemanager, used to add machines to state.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// ConstructClient is a constructor function for a machine manager client
func ConstructClient(clientFacade base.ClientFacade, facadeCaller base.FacadeCaller) *Client {
	return &Client{ClientFacade: clientFacade, facade: facadeCaller}
}

// NewClient returns a new machinemanager client.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, machineManagerFacade, options...)
	return ConstructClient(frontend, backend)
}

// ModelUUID returns the model UUID from the client connection.
func (c *Client) ModelUUID() (string, bool) {
	tag, ok := c.facade.RawAPICaller().ModelTag()
	return tag.Id(), ok
}

// AddMachines adds new machines with the supplied parameters, creating any requested disks.
func (client *Client) AddMachines(machineParams []params.AddMachineParams) ([]params.AddMachinesResult, error) {
	args := params.AddMachines{
		MachineParams: machineParams,
	}
	results := new(params.AddMachinesResults)

	err := client.facade.FacadeCall(context.TODO(), "AddMachines", args, results)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if len(results.Machines) != len(machineParams) {
		return nil, errors.Errorf("expected %d result, got %d", len(machineParams), len(results.Machines))
	}

	return results.Machines, nil
}

// DestroyMachinesWithParams removes the given set of machines, the semantics of which
// is determined by the force and keep parameters.
func (client *Client) DestroyMachinesWithParams(force, keep, dryRun bool, maxWait *time.Duration, machines ...string) ([]params.DestroyMachineResult, error) {
	args := params.DestroyMachinesParams{
		Force:       force,
		Keep:        keep,
		DryRun:      dryRun,
		MachineTags: make([]string, 0, len(machines)),
		MaxWait:     maxWait,
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
		if err := client.facade.FacadeCall(context.TODO(), "DestroyMachineWithParams", args, &result); err != nil {
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

// ProvisioningScript returns a shell script that, when run,
// provisions a machine agent on the machine executing the script.
func (c *Client) ProvisioningScript(args params.ProvisioningScriptParams) (script string, err error) {
	var result params.ProvisioningScriptResult
	if err = c.facade.FacadeCall(context.TODO(), "ProvisioningScript", args, &result); err != nil {
		return "", err
	}
	return result.Script, nil
}

// RetryProvisioning updates the provisioning status of a machine allowing the
// provisioner to retry.
func (c *Client) RetryProvisioning(all bool, machines ...names.MachineTag) ([]params.ErrorResult, error) {
	p := params.RetryProvisioningArgs{
		All: all,
	}
	p.Machines = make([]string, len(machines))
	for i, machine := range machines {
		p.Machines[i] = machine.String()
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(context.TODO(), "RetryProvisioning", p, &results)
	return results.Results, err
}
