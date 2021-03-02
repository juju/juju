// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package budget defines the command used to update budgets.
package budget

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/budget"
	"github.com/juju/utils"

	jujucmd "github.com/juju/juju/cmd"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
)

type budgetCommand struct {
	modelcmd.ModelCommandBase
	modelUUID string
	api       apiClient
	Wallet    string
	Limit     string
}

// NewBudgetCommand returns a new budgetCommand.
func NewBudgetCommand() cmd.Command {
	return modelcmd.Wrap(&budgetCommand{})
}

func (c *budgetCommand) newBudgetAPIClient(apiRoot string, bakery *httpbakery.Client) (apiClient, error) {
	if c.api != nil {
		return c.api, nil
	}
	var err error
	c.api, err = api.NewClient(api.APIRoot(apiRoot), api.HTTPClient(bakery))
	if err != nil {
		return nil, errors.Trace(err)
	}
	return c.api, nil
}

type apiClient interface {
	UpdateBudget(string, string, string) (string, error)
}

const doc = `
Updates an existing budget for a model.

Examples:
    # Sets the budget for the current model to 10.
    juju budget 10
    # Moves the budget for the current model to wallet 'personal' and sets the limit to 10.
    juju budget personal:10
`

// Info implements cmd.Command.Info.
func (c *budgetCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "budget",
		Args:    "[<wallet>:]<limit>",
		Purpose: "Update a budget.",
		Doc:     doc,
	})
}

func (c *budgetCommand) SetFlags(f *gnuflag.FlagSet) {
	f.StringVar(&c.modelUUID, "model-uuid", "", "Model uuid to set budget for.")
	c.ModelCommandBase.SetFlags(f)
}

func budgetDefinition(input string) (wallet, limit string, err error) {
	tokens := strings.Split(input, ":")
	switch len(tokens) {
	case 1:
		return "", tokens[0], nil
	case 2:
		return tokens[0], tokens[1], nil
	default:
		return "", "", errors.Errorf("invalid budget definition: %v", input)
	}
}

// Init implements cmd.Command.Init.
func (c *budgetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("value required")
	}

	wallet, limit, err := budgetDefinition(args[0])
	if err != nil {
		return errors.Trace(err)
	}
	if _, err := strconv.ParseInt(limit, 10, 32); err != nil {
		return errors.New("budget limit needs to be a whole number")
	}
	c.Wallet = wallet
	c.Limit = limit
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
	_, details, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return "", errors.Trace(err)
	}
	return details.ModelUUID, nil
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
	apiRoot, err := rcmd.GetMeteringURLForModelCmd(&c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	api, err := c.newBudgetAPIClient(apiRoot, client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.UpdateBudget(modelUUID, c.Wallet, c.Limit)
	if err != nil {
		return errors.Annotate(err, "failed to update the budget")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}
