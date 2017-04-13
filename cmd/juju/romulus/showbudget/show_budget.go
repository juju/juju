// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showbudget

import (
	"fmt"
	"io"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/loggo"
	api "github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

var logger = loggo.GetLogger("romulus.cmd.showbudget")

// NewShowBudgetCommand returns a new command that is used
// to show details of the specified wireformat.
func NewShowBudgetCommand() cmd.Command {
	return modelcmd.WrapBase(&showBudgetCommand{})
}

type showBudgetCommand struct {
	modelcmd.JujuCommandBase

	out    cmd.Output
	budget string
}

const showBudgetDoc = `
Display budget usage information.

Examples:
    juju show-budget personal
`

// Info implements cmd.Command.Info.
func (c *showBudgetCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "show-budget",
		Args:    "<budget>",
		Purpose: "Show details about a budget.",
		Doc:     showBudgetDoc,
	}
}

// Init implements cmd.Command.Init.
func (c *showBudgetCommand) Init(args []string) error {
	if len(args) < 1 {
		return errors.New("missing arguments")
	}
	c.budget, args = args[0], args[1:]

	return c.JujuCommandBase.Init(args)
}

// SetFlags implements cmd.Command.SetFlags.
func (c *showBudgetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"tabular": formatTabular,
		"json":    cmd.FormatJson,
	})
}

func (c *showBudgetCommand) Run(ctx *cmd.Context) error {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}
	api, err := newBudgetAPIClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create an api client")
	}
	budget, err := api.GetBudget(c.budget)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve the budget")
	}
	err = c.out.Write(ctx, budget)
	return errors.Trace(err)
}

// formatTabular returns a tabular view of available budgets.
func formatTabular(writer io.Writer, value interface{}) error {
	b, ok := value.(*wireformat.BudgetWithAllocations)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", b, value)
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true
	for _, col := range []int{2, 3, 5} {
		table.RightAlign(col)
	}

	table.AddRow("Model", "Spent", "Allocated", "By", "Usage")
	for _, allocation := range b.Allocations {
		table.AddRow(allocation.Model, allocation.Consumed, allocation.Limit, allocation.Owner, allocation.Usage)
	}
	table.AddRow("", "", "", "")
	table.AddRow("Total", b.Total.Consumed, b.Total.Allocated, "", b.Total.Usage)
	table.AddRow("Budget", "", b.Limit, "")
	table.AddRow("Unallocated", "", b.Total.Unallocated, "")
	fmt.Fprint(writer, table)
	return nil
}

type APIClient interface {
	ModelInfo(tags []names.ModelTag) ([]params.ModelInfoResult, error)
}

var newBudgetAPIClient = newBudgetAPIClientImpl

func newBudgetAPIClientImpl(c *httpbakery.Client) (budgetAPIClient, error) {
	client := api.NewClient(c)
	return client, nil
}

type budgetAPIClient interface {
	GetBudget(string) (*wireformat.BudgetWithAllocations, error)
}
