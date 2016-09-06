// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package showbudget

import (
	"fmt"
	"io"
	"sort"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/juju/api/modelmanager"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	api "github.com/juju/romulus/api/budget"
	wireformat "github.com/juju/romulus/wireformat/budget"
)

var logger = loggo.GetLogger("romulus.cmd.showbudget")

// NewShowBudgetCommand returns a new command that is used
// to show details of the specified wireformat.
func NewShowBudgetCommand() modelcmd.CommandBase {
	return &showBudgetCommand{}
}

type showBudgetCommand struct {
	modelcmd.ModelCommandBase

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

	return cmd.CheckEmpty(args)
}

// SetFlags implements cmd.Command.SetFlags.
func (c *showBudgetCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
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
	c.resolveModelNames(budget)
	err = c.out.Write(ctx, budget)
	return errors.Trace(err)
}

// resolveModelNames is a best-effort method to resolve model names - if we
// encounter any error, we do not issue an error.
func (c *showBudgetCommand) resolveModelNames(budget *wireformat.BudgetWithAllocations) {
	models := make([]names.ModelTag, len(budget.Allocations))
	for i, allocation := range budget.Allocations {
		models[i] = names.NewModelTag(allocation.Model)
	}
	client, err := newAPIClient(c)
	if err != nil {
		logger.Errorf("failed to open the API client: %v", err)
		return
	}
	modelInfoSlice, err := client.ModelInfo(models)
	if err != nil {
		logger.Errorf("failed to retrieve model info: %v", err)
		return
	}
	for j, info := range modelInfoSlice {
		if info.Error != nil {
			logger.Errorf("failed to get model info for model %q: %v", models[j], info.Error)
			continue
		}
		for i, allocation := range budget.Allocations {
			if info.Result.UUID == allocation.Model {
				budget.Allocations[i].Model = info.Result.Name
			}
		}
	}
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

	table.AddRow("MODEL", "SERVICES", "SPENT", "ALLOCATED", "BY", "USAGE")
	for _, allocation := range b.Allocations {
		firstLine := true
		// We'll sort the service names to avoid nondeterministic
		// command output.
		services := make([]string, 0, len(allocation.Services))
		for serviceName, _ := range allocation.Services {
			services = append(services, serviceName)
		}
		sort.Strings(services)
		for _, serviceName := range services {
			service, _ := allocation.Services[serviceName]
			if firstLine {
				table.AddRow(allocation.Model, serviceName, service.Consumed, allocation.Limit, allocation.Owner, allocation.Usage)
				firstLine = false
				continue
			}
			table.AddRow("", serviceName, service.Consumed, "", "")
		}

	}
	table.AddRow("", "", "", "", "")
	table.AddRow("TOTAL", "", b.Total.Consumed, b.Total.Allocated, "", b.Total.Usage)
	table.AddRow("BUDGET", "", "", b.Limit, "")
	table.AddRow("UNALLOCATED", "", "", b.Total.Unallocated, "")
	fmt.Fprint(writer, table)
	return nil
}

var newAPIClient = newAPIClientImpl

func newAPIClientImpl(c *showBudgetCommand) (APIClient, error) {
	root, err := c.NewControllerAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return modelmanager.NewClient(root), nil
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
