// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/charmhub"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	findSummary = "Queries the charmhub store for available charms or bundles."
	findDoc     = `
The find command queries the charmhub store for available charms or bundles.

Examples:
    juju find wordpress

See also:
    info
`
)

// NewFindCommand wraps findCommand with sane model settings.
func NewFindCommand() cmd.Command {
	return modelcmd.Wrap(&findCommand{})
}

// findCommand supplies the "find" CLI command used to display find information.
type findCommand struct {
	modelcmd.ModelCommandBase

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
	c.ModelCommandBase.SetFlags(f)
	// TODO (stickupkid): add the following:
	// --narrow
}

// Init initializes the find command, including validating the provided
// flags. It implements part of the cmd.Command interface.
func (c *findCommand) Init(args []string) error {
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
	if err != nil {
		return err
	}

	// If the results are empty, we should return a helpful message to the
	// operator.
	if len(results) == 0 {
		fmt.Fprintf(ctx.Stderr, "No matching charms for %q\n", c.query)
		return nil
	}

	// Output some helpful errors messages for operators if the query is empty
	// or not.
	emptyQuery := c.query == ""
	if emptyQuery {
		fmt.Fprintf(ctx.Stdout, "No search term specified. Here are some interesting charms:\n\n")
	}

	if err := makeFindWriter(ctx, results).Print(); err != nil {
		return errors.Trace(err)
	}

	if emptyQuery {
		fmt.Fprintln(ctx.Stdout, "Provide a search term for more specific results.")
	}
	return nil
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
