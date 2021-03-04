// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package createwallet

import (
	"fmt"
	"strconv"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	api "github.com/juju/romulus/api/budget"

	jujucmd "github.com/juju/juju/cmd"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
)

type createWalletCommand struct {
	modelcmd.ControllerCommandBase
	Name  string
	Value string
}

// NewCreateWalletCommand returns a new createWalletCommand
func NewCreateWalletCommand() modelcmd.ControllerCommand {
	return modelcmd.WrapController(&createWalletCommand{})
}

const doc = `
Create a new wallet with monthly limit.

Examples:
    # Creates a wallet named 'qa' with a limit of 42.
    juju create-wallet qa 42
`

// Info implements cmd.Command.Info.
func (c *createWalletCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "create-wallet",
		Purpose: "Create a new wallet.",
		Doc:     doc,
	})
}

// Init implements cmd.Command.Init.
func (c *createWalletCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("name and value required")
	}
	c.Name, c.Value = args[0], args[1]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("wallet value needs to be a whole number")
	}
	return c.CommandBase.Init(args[2:])
}

// Run implements cmd.Command.Run and has most of the logic for the run command.
func (c *createWalletCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	apiRoot, err := rcmd.GetMeteringURLForControllerCmd(&c.ControllerCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	api, err := newAPIClient(apiRoot, client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.CreateWallet(c.Name, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to create the wallet")
	}
	fmt.Fprintln(ctx.Stdout, resp)
	return nil
}

var newAPIClient = newAPIClientImpl

func newAPIClientImpl(apiRoot string, c *httpbakery.Client) (apiClient, error) {
	return api.NewClient(api.APIRoot(apiRoot), api.HTTPClient(c))
}

type apiClient interface {
	CreateWallet(name string, limit string) (string, error)
}
