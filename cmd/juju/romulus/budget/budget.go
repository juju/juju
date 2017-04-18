// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package budget defines the command used to update budgets.
package budget

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

type budgetCommand struct {
	modelcmd.ModelCommandBase
	modelUUID string
	api       apiClient
	Value     string
}

// NewBudgetCommand returns a new budgetCommand.
func NewBudgetCommand() cmd.Command {
	return modelcmd.Wrap(&budgetCommand{})
}

func (c *budgetCommand) newAPIClient(bakery *httpbakery.Client) (apiClient, error) {
	if c.api != nil {
		return c.api, nil
	}
	c.api = api.NewClient(bakery)
	return c.api, nil
}

type apiClient interface {
	UpdateBudget(string, string) (string, error)
}

const doc = `
Updates an existing budget for a model.

Examples:
    # Sets the budget for the current model to 10.
    juju budget 10
`

// Info implements cmd.Command.Info.
func (c *budgetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "budget",
		Args:    "<value>",
		Purpose: "Update a budget.",
		Doc:     doc,
	}
}

func (c *budgetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.modelUUID, "model-uuid", "", "Model uuid to set budget for.")
	c.ModelCommandBase.SetFlags(f)
}

// Init implements cmd.Command.Init.
func (c *budgetCommand) Init(args []string) error {
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

func (c *budgetCommand) getModelUUID() (string, error) {
	if c.modelUUID != "" {
		return c.modelUUID, nil
	}
	model, err := c.ClientStore().ModelByName(c.ControllerName(), c.ModelName())
	if err != nil {
		return "", errors.Trace(err)
	}
	return model.ModelUUID, nil
}

// Run implements cmd.Command.Run and contains most of the setwallet logic.
func (c *budgetCommand) Run(ctx *cmd.Context) error {
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
	resp, err := api.UpdateBudget(modelUUID, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to update the budget")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}
