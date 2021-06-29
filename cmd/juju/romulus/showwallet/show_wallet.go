// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showwallet

import (
	"fmt"
	"io"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo/v2"
	api "github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"

	jujucmd "github.com/juju/juju/cmd"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
)

var logger = loggo.GetLogger("romulus.cmd.showwallet")

// NewShowWalletCommand returns a new command that is used
// to show details of the specified wireformat.
func NewShowWalletCommand() modelcmd.ControllerCommand {
	return modelcmd.WrapController(&showWalletCommand{})
}

type showWalletCommand struct {
	modelcmd.ControllerCommandBase

	out    cmd.Output
	wallet string
}

const showWalletDoc = `
Display wallet usage information.

Examples:
    juju show-wallet personal
`

// Info implements cmd.Command.Info.
func (c *showWalletCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-wallet",
		Args:    "<wallet>",
		Purpose: "Show details about a wallet.",
		Doc:     showWalletDoc,
	})
}

// Init implements cmd.Command.Init.
func (c *showWalletCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing arguments")
	}
	c.wallet, args = args[0], args[1:]

	return c.CommandBase.Init(args)
}

// SetFlags implements cmd.Command.SetFlags.
func (c *showWalletCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": c.formatTabular,
		"json":    cmd.FormatJson,
	})
}

func (c *showWalletCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	apiRoot, err := rcmd.GetMeteringURLForControllerCmd(&c.ControllerCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	api, err := newWalletAPIClient(apiRoot, client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	wallet, err := api.GetWallet(c.wallet)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve the wallet")
	}
	err = c.out.Write(ctx, wallet)
	return errors.Trace(err)
}

// formatTabular returns a tabular view of available wallets.
func (c *showWalletCommand) formatTabular(writer io.Writer, value interface{}) error {
	b, ok := value.(*wireformat.WalletWithBudgets)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", b, value)
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{2, 3, 5} {
		table.RightAlign(col)
	}

	uuidToModelName, err := c.modelNameMap()
	if err != nil {
		logger.Warningf("failed to read juju client model names")
		uuidToModelName = map[string]string{}
	}
	table.AddRow("Model", "Spent", "Budgeted", "By", "Usage")
	for _, budget := range b.Budgets {
		modelName := uuidToModelName[budget.Model]
		if modelName == "" {
			modelName = budget.Model
		}
		table.AddRow(modelName, budget.Consumed, budget.Limit, budget.Owner, budget.Usage)
	}
	table.AddRow("", "", "", "")
	table.AddRow("Total", b.Total.Consumed, b.Total.Budgeted, "", b.Total.Usage)
	table.AddRow("Wallet", "", b.Limit, "")
	table.AddRow("Unallocated", "", b.Total.Unallocated, "")
	fmt.Fprint(writer, table)
	return nil
}

func (c *showWalletCommand) modelNameMap() (map[string]string, error) {
	store := newJujuclientStore()
	uuidToName := map[string]string{}
	controllers, err := store.AllControllers()
	if err != nil {
		return nil, errors.Trace(err)
	}
	for cname := range controllers {
		models, err := store.AllModels(cname)
		if err != nil {
			return nil, errors.Trace(err)
		}
		for mname, mdetails := range models {
			uuidToName[mdetails.ModelUUID] = cname + ":" + mname
		}
	}
	return uuidToName, nil
}

var newWalletAPIClient = newWalletAPIClientImpl

func newWalletAPIClientImpl(apiRoot string, c *httpbakery.Client) (walletAPIClient, error) {
	return api.NewClient(api.APIRoot(apiRoot), api.HTTPClient(c))
}

type walletAPIClient interface {
	GetWallet(string) (*wireformat.WalletWithBudgets, error)
}

var newJujuclientStore = jujuclient.NewFileClientStore
