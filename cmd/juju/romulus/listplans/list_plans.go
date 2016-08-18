// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The listplans package contains implementation of the command that
// can be used to list plans that are available for a charm.
package listplans

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/gosuri/uitable"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/plan"
	wireformat "github.com/juju/romulus/wireformat/plan"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/yaml.v2"

	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// apiClient defines the interface of the plan api client need by this command.
type apiClient interface {
	// GetAssociatedPlans returns the plans associated with the charm.
	GetAssociatedPlans(charmURL string) ([]wireformat.Plan, error)
}

var newClient = func(client *httpbakery.Client) (apiClient, error) {
	return api.NewClient(api.HTTPClient(client))
}

const listPlansDoc = `
List plans available for the specified charm.

Examples:
    juju plans cs:webapp
`

// ListPlansCommand retrieves plans that are available for the specified charm
type ListPlansCommand struct {
	modelcmd.JujuCommandBase

	out      cmd.Output
	CharmURL string

	CharmResolver rcmd.CharmResolver
}

// NewListPlansCommand creates a new ListPlansCommand.
func NewListPlansCommand() modelcmd.CommandBase {
	return &ListPlansCommand{
		CharmResolver: rcmd.NewCharmStoreResolver(),
	}
}

// Info implements Command.Info.
func (c *ListPlansCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "plans",
		Args:    "",
		Purpose: "List plans.",
		Doc:     listPlansDoc,
		Aliases: []string{"list-plans"},
	}
}

// Init reads and verifies the cli arguments for the ListPlansCommand
func (c *ListPlansCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing arguments")
	}
	charmURL, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Errorf("unknown command line arguments: " + strings.Join(args, ","))
	}
	c.CharmURL = charmURL
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *ListPlansCommand) SetFlags(f *gnuflag.FlagSet) {
	c.JujuCommandBase.SetFlags(f)
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"smart":   cmd.FormatSmart,
		"summary": formatSummary,
		"tabular": formatTabular,
	})
}

// Run implements Command.Run.
// Retrieves the plan from the plans service. The set of plans to be
// retrieved can be limited using the plan and isv flags.
func (c *ListPlansCommand) Run(ctx *cmd.Context) (rErr error) {
	client, err := c.BakeryClient()
	if err != nil {
		return errors.Annotate(err, "failed to create an http client")
	}

	resolvedUrl, err := c.CharmResolver.Resolve(client.VisitWebPage, client.Client, c.CharmURL)
	if err != nil {
		return errors.Annotatef(err, "failed to resolve charmURL %v", c.CharmURL)
	}
	c.CharmURL = resolvedUrl

	apiClient, err := newClient(client)
	if err != nil {
		return errors.Annotate(err, "failed to create a plan API client")
	}

	plans, err := apiClient.GetAssociatedPlans(c.CharmURL)
	if err != nil {
		return errors.Annotate(err, "failed to retrieve plans")
	}

	output := make([]plan, len(plans))
	for i, p := range plans {
		outputPlan := plan{
			URL: p.URL,
		}
		def, err := readPlan(bytes.NewBufferString(p.Definition))
		if err != nil {
			return errors.Annotate(err, "failed to parse plan definition")
		}
		if def.Description != nil {
			outputPlan.Price = def.Description.Price
			outputPlan.Description = def.Description.Text
		}
		output[i] = outputPlan
	}
	err = c.out.Write(ctx, output)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

type plan struct {
	URL         string `json:"plan" yaml:"plan"`
	Price       string `json:"price" yaml:"price"`
	Description string `json:"description" yaml:"description"`
}

// formatSummary returns a summary of available plans.
func formatSummary(writer io.Writer, value interface{}) error {
	plans, ok := value.([]plan)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", plans, value)
	}
	tw := output.TabWriter(writer)
	p := func(values ...interface{}) {
		for _, v := range values {
			fmt.Fprintf(tw, "%s\t", v)
		}
		fmt.Fprintln(tw)
	}
	p("PLAN", "PRICE")
	for _, plan := range plans {
		p(plan.URL, plan.Price)
	}
	tw.Flush()
	return nil
}

// formatTabular returns a tabular summary of available plans.
func formatTabular(writer io.Writer, value interface{}) error {
	plans, ok := value.([]plan)
	if !ok {
		return errors.Errorf("expected value of type %T, got %T", plans, value)
	}

	table := uitable.New()
	table.MaxColWidth = 50
	table.Wrap = true

	table.AddRow("PLAN", "PRICE", "DESCRIPTION")
	for _, plan := range plans {
		table.AddRow(plan.URL, plan.Price, plan.Description)
	}
	fmt.Fprint(writer, table)
	return nil
}

type planModel struct {
	Description *descriptionModel `json:"description,omitempty"`
}

// descriptionModel provides a human readable description of the plan.
type descriptionModel struct {
	Price string `json:"price,omitempty"`
	Text  string `json:"text,omitempty"`
}

// readPlan reads, parses and returns a planModel struct representation.
func readPlan(r io.Reader) (plan *planModel, err error) {
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return
	}

	var doc planModel
	err = yaml.Unmarshal(data, &doc)
	if err != nil {
		return
	}
	return &doc, nil
}
