// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
)

const (
	findSummary = "Queries the CharmHub store for available charms or bundles."
	findDoc     = `
The find command queries the CharmHub store for available charms or bundles.

Examples:
    juju find wordpress

See also:
    info
    download
`
)

// NewFindCommand wraps findCommand with sane model settings.
func NewFindCommand() cmd.Command {
	return modelcmd.Wrap(&findCommand{
		charmhubCommand: &charmhubCommand{
			arches: arch.AllArches(),
		},
	})
}

// findCommand supplies the "find" CLI command used to display find information.
type findCommand struct {
	*charmhubCommand

	out        cmd.Output
	warningLog Log

	api FindCommandAPI

	query string
}

// Find returns help related info about the command, it implements
// part of the cmd.Command interface.
func (c *findCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "find",
		Args:    "[options] <query>",
		Purpose: findSummary,
		Doc:     findDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags defines flags which can be used with the find command.
// It implements part of the cmd.Command interface.
func (c *findCommand) SetFlags(f *gnuflag.FlagSet) {
	c.charmhubCommand.SetFlags(f)

	c.out.AddFlags(f, "tabular", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.formatter,
	})
	// TODO (stickupkid): add the following:
	// --narrow
}

// Init initializes the find command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *findCommand) Init(args []string) error {
	if err := c.charmhubCommand.Init(args); err != nil {
		return errors.Trace(err)
	}

	// We allow searching of empty queries, which will return a list of
	// "interesting charms".
	if len(args) > 0 {
		c.query = args[0]
	}

	return nil
}

// Run is the business logic of the find command.  It implements the meaty
// part of the cmd.Command interface.
func (c *findCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	results, err := client.Find(c.query)
	if params.IsCodeNotFound(err) {
		return errors.Wrap(err, errors.Errorf("Nothing found for query %q.", c.query))
	} else if err != nil {
		return errors.Trace(err)
	}

	// This is a side effect of the formatting code not wanting to error out
	// when we get invalid data from the API.
	// We store it on the command before attempting to output, so we can pick
	// it up later.
	c.warningLog = ctx.Warningf

	results = filterFindResults(results, c.arch, c.series)

	return c.output(ctx, results)
}

// getAPI returns the API that supplies methods
// required to execute this command.
func (c *findCommand) getAPI() (FindCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := charmhub.NewClient(api)
	return client, nil
}

func (c *findCommand) output(ctx *cmd.Context, results []charmhub.FindResponse) error {
	tabular := c.out.Name() == "tabular"
	if tabular {
		// If the results are empty, we should return a helpful message to the
		// operator.
		if len(results) == 0 {
			fmt.Fprintf(ctx.Stderr, "No matching charms for %q\n", c.query)
			return nil
		}

		// Output some helpful errors messages for operators if the query is empty
		// or not.
		if c.query == "" {
			fmt.Fprintf(ctx.Stdout, "No search term specified. Here are some interesting charms:\n\n")
		}
	}

	view := convertCharmFindResults(results)
	if err := c.out.Write(ctx, view); err != nil {
		return errors.Trace(err)
	}

	if tabular && c.query == "" {
		fmt.Fprintln(ctx.Stdout, "Provide a search term for more specific results.")
	}

	return nil
}

func (c *findCommand) formatter(writer io.Writer, value interface{}) error {
	results, ok := value.([]FindResponse)
	if !ok {
		return errors.Errorf("unexpected results")
	}

	if err := makeFindWriter(writer, c.warningLog, results).Print(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
