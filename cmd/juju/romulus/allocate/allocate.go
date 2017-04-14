// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package allocate defines the command used to update allocations.
package allocate

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/budget"
	"github.com/juju/utils"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

type allocateCommand struct {
	modelcmd.ModelCommandBase
	modelUUID string
	api       apiClient
	Value     string
}

// NewAllocateCommand returns a new allocateCommand.
func NewAllocateCommand() cmd.Command {
	return modelcmd.Wrap(&allocateCommand{})
}

func (c *allocateCommand) newAPIClient(bakery *httpbakery.Client) (apiClient, error) {
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
    juju allocate 10
`

// Info implements cmd.Command.Info.
func (c *allocateCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "allocate",
		Args:    "<value>",
		Purpose: "Update an allocation.",
		Doc:     doc,
	}
}

func (c *allocateCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.modelUUID, "model-uuid", "", "Model uuid to set allocation for.")
	c.ModelCommandBase.SetFlags(f)
}

// Init implements cmd.Command.Init.
func (c *allocateCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("value required")
	}
	c.Value = args[0]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("value needs to be a whole number")
	}
	if c.modelUUID != "" {
		if !utils.IsValidUUIDString(c.modelUUID) {
			return errors.NotValidf("provided model UUID %q", c.modelUUID)
		}
	}
	return c.ModelCommandBase.Init(args[1:])
}

func (c *allocateCommand) getModelUUID() (string, error) {
	if c.modelUUID != "" {
		return c.modelUUID, nil
	}
	model, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName())
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.ModelUUID, nil
}

// Run implements cmd.Command.Run and contains most of the setbudget logic.
func (c *allocateCommand) Run(ctx *cmd.Context) error {
	modelUUID, err := c.getModelUUID()
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
