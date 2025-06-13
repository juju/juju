// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package listplans

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"strings"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/gosuri/uitable"
	"github.com/juju/charm/v8"
	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	api "github.com/juju/romulus/api/plan"
	wireformat "github.com/juju/romulus/wireformat/plan"
	"gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
	rcmd "github.com/juju/juju/cmd/juju/romulus"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

// apiClient defines the interface of the plan api client need by this command.
type apiClient interface {
	// GetAssociatedPlans returns the plans associated with the charm.
	GetAssociatedPlans(charmURL string) ([]wireformat.Plan, error)
}

var newClient = func(apiRoot string, client *httpbakery.Client) (apiClient, error) {
	return api.NewClient(api.APIRoot(apiRoot), api.HTTPClient(client))
}

const listPlansDoc = `
List plans available for the specified charm.

Examples:
    juju plans cs:webapp
`

// ListPlansCommand retrieves plans that are available for the specified charm
type ListPlansCommand struct {
	modelcmd.ControllerCommandBase

	out      cmd.Output
	CharmURL string
}

// NewListPlansCommand creates a new ListPlansCommand.
func NewListPlansCommand() modelcmd.ControllerCommand {
	return modelcmd.WrapController(&ListPlansCommand{})
}

// Info implements Command.Info.
func (c *ListPlansCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "plans",
		Args:    "<charm-url>",
		Purpose: "List plans.",
		Doc:     listPlansDoc,
		Aliases: []string{"list-plans"},
	})
}

// Init reads and verifies the cli arguments for the ListPlansCommand
func (c *ListPlansCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("missing charm-store charm URL argument")
	}
	charmURL, args := args[0], args[1:]
	if err := cmd.CheckEmpty(args); err != nil {
		return errors.Errorf("unknown command line arguments: %s", strings.Join(args, ","))
	}
	curl, err := charm.ParseURL(charmURL)
	if err != nil {
		return errors.Annotatef(err, "unable to parse charm URL")
	}
	if !charm.CharmStore.Matches(curl.Schema) {
		return errors.NotSupportedf("non charm-store URLs")
	}
	c.CharmURL = charmURL
	return c.CommandBase.Init(args)
}

// SetFlags implements Command.SetFlags.
func (c *ListPlansCommand) SetFlags(f *gnuflag.FlagSet) {
	c.CommandBase.SetFlags(f)
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

	resolver, err := rcmd.NewCharmStoreResolverForControllerCmd(&c.ControllerCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	resolvedURL, err := resolver.Resolve(client, c.CharmURL)
	if err != nil {
		return errors.Annotatef(err, "failed to resolve charmURL %v", c.CharmURL)
	}
	c.CharmURL = resolvedURL

	apiRoot, err := rcmd.GetMeteringURLForControllerCmd(&c.ControllerCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	apiClient, err := newClient(apiRoot, client)
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

	if len(output) == 0 && c.out.Name() == "tabular" {
		ctx.Infof("No plans to display.")
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
	p("Plan", "Price")
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

	table.AddRow("Plan", "Price", "Description")
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
