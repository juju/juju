// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package updateallocation defines the command used to update allocations.
package updateallocation

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	api "github.com/juju/romulus/api/budget"
)

type updateAllocationCommand struct {
	modelcmd.ModelCommandBase
	api   apiClient
	Name  string
	Value string
}

// NewUpdateAllocationCommand returns a new updateAllocationCommand.
func NewUpdateAllocationCommand() modelcmd.ModelCommand {
	return &updateAllocationCommand{}
}

func (c *updateAllocationCommand) newAPIClient(bakery *httpbakery.Client) (apiClient, error) {
	if c.api != nil {
		return c.api, nil
	}
	c.api = api.NewClient(bakery)
	return c.api, nil
}

type apiClient interface {
	UpdateAllocation(string, string, string) (string, error)
}

const doc = `
Updates an existing allocation on an application.

Examples:
    # Sets the allocation for the wordpress application to 10.
    juju update-allocation wordpress 10
`

// Info implements cmd.Command.Info.
func (c *updateAllocationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "update-allocation",
		Args:    "<application> <value>",
		Purpose: "Update an allocation.",
		Doc:     doc,
	}
}

// Init implements cmd.Command.Init.
func (c *updateAllocationCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("application and value required")
	}
	c.Name, c.Value = args[0], args[1]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("value needs to be a whole number")
	}
	return cmd.CheckEmpty(args[2:])
}

func (c *updateAllocationCommand) modelUUID() (string, error) {
	model, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName())
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.ModelUUID, nil
}

// Run implements cmd.Command.Run and contains most of the setbudget logic.
func (c *updateAllocationCommand) Run(ctx *cmd.Context) error {
	modelUUID, err := c.modelUUID()
	if err != nil {
		return errors.Annotate(err, "failed to get model uuid")
	}
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := c.newAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.UpdateAllocation(modelUUID, c.Name, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to update the allocation")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}
