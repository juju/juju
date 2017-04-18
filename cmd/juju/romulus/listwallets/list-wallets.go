// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listwallets

import (
	"fmt"
	"io"
	"sort"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/cmd/modelcmd"
)

// NewListWalletsCommand returns a new command that is used
// to list wallets a user has access to.
func NewListWalletsCommand() cmd.Command {
	return modelcmd.WrapBase(&listWalletsCommand{})
}

type listWalletsCommand struct {
	modelcmd.JujuCommandBase

	out cmd.Output
}

const listWalletsDoc = `
List the available wallets.

Examples:
    juju wallets
`

// Info implements cmd.Command.Info.
func (c *listWalletsCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "wallets",
		Purpose: "List wallets.",
		Doc:     listWalletsDoc,
		Aliases: []string{"list-wallets"},
	}
}

// SetFlags implements cmd.Command.SetFlags.
func (c *listWalletsCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": formatTabular,
		"json":    cmd.FormatJson,
	})
}

func (c *listWalletsCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := newAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	wallets, err := api.ListWallets()
	if err != nil {
		return errors.Annotate(err, "failed to retrieve wallets")
	}
	if wallets == nil {
		return errors.New("no wallet information available")
	}
	err = c.out.Write(ctx, wallets)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// formatTabular returns a tabular view of available wallets.
func formatTabular(writer io.Writer, value interface{}) error {
	b, ok := value.(*wireformat.ListWalletsResponse)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", b, value)
	}
	sort.Sort(b.Wallets)

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{1, 2, 3, 4} {
		table.RightAlign(col)
	}

	table.AddRow("Wallet", "Monthly", "Allocated", "Available", "Spent")
	for _, walletEntry := range b.Wallets {
		suffix := ""
		if walletEntry.Default {
			suffix = "*"
		}
		table.AddRow(walletEntry.Wallet+suffix, walletEntry.Limit, walletEntry.Allocated, walletEntry.Available, walletEntry.Consumed)
	}
	table.AddRow("Total", b.Total.Limit, b.Total.Allocated, b.Total.Available, b.Total.Consumed)
	table.AddRow("", "", "", "", "")
	table.AddRow("Credit limit:", b.Credit, "", "", "")
	fmt.Fprint(writer, table)
	return nil
}

var newAPIClient = newAPIClientImpl

func newAPIClientImpl(c *httpbakery.Client) (apiClient, error) {
	client := api.NewClient(c)
	return client, nil
}

type apiClient interface {
	// ListWallets returns a list of wallets a user has access to.
	ListWallets() (*wireformat.ListWalletsResponse, error)
}
