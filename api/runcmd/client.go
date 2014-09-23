package runcmd

import (
	"time"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the juju run command, used to execute
// commands for a given machine, unit, and/or service.
type Client struct {
	base.ClientFacade
	facade     base.FacadeCaller
	environTag string
}

// NewClient returns a new RunCommand client.
func NewClient(caller base.APICallCloser, environTag string) *Client {
	frontend, backend := base.NewClientFacade(caller, "RunCommand")
	return &Client{ClientFacade: frontend, facade: backend, environTag: environTag}
}

// RunOnAllMachines runs the command on all the machines with the specified
// timeout.
func (c *Client) RunOnAllMachines(commands string, timeout time.Duration) ([]params.RunResult, error) {
	var results params.RunResults
	args := params.RunParams{Commands: commands, Timeout: timeout}
	err := c.facade.FacadeCall("RunOnAllMachines", args, &results)
	return results.Results, err
}

// Run the Commands specified on the machines identified through the ids
// provided in the machines, services and units slices.
func (c *Client) Run(run params.RunParams) ([]params.RunResult, error) {
	var results params.RunResults
	err := c.facade.FacadeCall("Run", run, &results)
	return results.Results, err
}
