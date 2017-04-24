// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setwallet

import (
	"fmt"
	"strconv"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	api "github.com/juju/romulus/api/budget"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

type setWalletCommand struct {
	modelcmd.JujuCommandBase
	Name  string
	Value string
}

// NewSetWalletCommand returns a new setWalletCommand.
func NewSetWalletCommand() cmd.Command {
	return modelcmd.WrapBase(&setWalletCommand{})
}

const doc = `
Set the monthly wallet limit.

Examples:
    # Sets the monthly limit for wallet named 'personal' to 96.
    juju set-wallet personal 96
`

// Info implements cmd.Command.Info.
func (c *setWalletCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "set-wallet",
		Args:    "<wallet name> <value>",
		Purpose: "Set the wallet limit.",
		Doc:     doc,
	}
}

// Init implements cmd.Command.Init.
func (c *setWalletCommand) Init(args []string) error {
	if len(args) < 2 {
		return errors.New("name and value required")
	}
	c.Name, c.Value = args[0], args[1]
	if _, err := strconv.ParseInt(c.Value, 10, 32); err != nil {
		return errors.New("wallet value needs to be a whole number")
	}
	return c.JujuCommandBase.Init(args[2:])
}

// Run implements cmd.Command.Run and contains most of the setwallet logic.
func (c *setWalletCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := newAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	resp, err := api.SetWallet(c.Name, c.Value)
	if err != nil {
		return errors.Annotate(err, "failed to set the wallet")
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
	SetWallet(string, string) (string, error)
}
