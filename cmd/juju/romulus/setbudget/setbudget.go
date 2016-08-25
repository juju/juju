// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setbudget

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/cmd/modelcmd"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	api "github.com/juju/romulus/api/budget"
)

type setBudgetCommand struct {
	modelcmd.JujuCommandBase
	Name  string
	Value string
}

// NewSetBudgetCommand returns a new setBudgetCommand.
func NewSetBudgetCommand() cmd.Command {
	return &setBudgetCommand{}
}

const doc = `
Set the monthly budget limit.

Examples:
    # Sets the monthly limit for budget named 'personal' to 96.
    juju set-budget personal 96
`

// Info implements cmd.Command.Info.
func (c *setBudgetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-budget",
		Args:    "<budget name> <value>",
		Purpose: "Set the budget limit.",
		Doc:     doc,
	}
}

// Init implements cmd.Command.Init.
func (c *setBudgetCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("name and value required")
	}
	c.Name, c.Value = args[0], args[1]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("budget value needs to be a whole number")
	}
	return cmd.CheckEmpty(args[2:])
}

// Run implements cmd.Command.Run and contains most of the setbudget logic.
func (c *setBudgetCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := newAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.SetBudget(c.Name, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to set the budget")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}

var newAPIClient = newAPIClientImpl

func newAPIClientImpl(c *httpbakery.Client) (apiClient, error) {
	client := api.NewClient(c)
	return client, nil
}

type apiClient interface {
	SetBudget(string, string) (string, error)
}
