package runcmd

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

const runcmdFacade = "RunCommand"

// Client provides access to the juju run command, used to execute
// commands for a given machine, unit, and/or service.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new RunCommand client.
func NewClient(caller base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(caller, runcmdFacade)
	return &Client{ClientFacade: frontend, facade: backend}
}

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(run params.RunParamsV1) ([]params.RunResult, error) {
	var results params.RunResults
	args := params.RunParams{Commands: run.Commands, Timeout: run.Timeout}
	err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
	return results.Results, err
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, services and units slices.
func (c *Client) Run(run params.RunParamsV1) ([]params.RunResult, error) {
	if c.facade.BestAPIVersion() < 1 {
		if run.Context != nil {
			if len(run.Context.Relation) > 0 || len(run.Context.RemoteUnit) > 0 {
				return nil, errors.NotImplementedf("The server does not support the supplied option(s): --relation, --remote-unit. (apiversion: %d)", c.facade.BestAPIVersion())
			}
		}
	}
	var results params.RunResults
	err := c.facade.FacadeCall("Run", run, &results)
	return results.Results, err
}
