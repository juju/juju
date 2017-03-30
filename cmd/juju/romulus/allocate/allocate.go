// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package allocate defines the command used to update allocations.
package allocate

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	api "github.com/juju/romulus/api/budget"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

type updateAllocationCommand struct {
	modelcmd.ModelCommandBase
	api   apiClient
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
	UpdateAllocation(string, string) (string, error)
}

const doc = `
Updates an existing allocation for a model.

Examples:
    # Sets the allocation for the current model to 10.
    juju update-allocation 10
`

// Info implements cmd.Command.Info.
func (c *updateAllocationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "allocate",
		Args:    "<value>",
		Purpose: "Update an allocation.",
		Doc:     doc,
	}
}

// Init implements cmd.Command.Init.
func (c *updateAllocationCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("value required")
	}
	c.Value = args[0]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("value needs to be a whole number")
	}
	return cmd.CheckEmpty(args[1:])
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
	resp, err := api.UpdateAllocation(modelUUID, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to update the allocation")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}
