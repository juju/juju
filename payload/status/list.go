// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status

import (
	"fmt"
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/payload"
)

// ListAPI has the API methods needed by ListCommand.
type ListAPI interface {
	ListFull(patterns ...string) ([]payload.FullPayloadInfo, error)
	io.Closer
}

// ListCommand implements the list-payloads command.
type ListCommand struct {
	modelcmd.ModelCommandBase
	out      cmd.Output
	patterns []string

	newAPIClient func(c *ListCommand) (ListAPI, error)
}

// NewListCommand returns a new command that lists charm payloads
// in the current environment.
func NewListCommand(newAPIClient func(c *ListCommand) (ListAPI, error)) *ListCommand {
	cmd := &ListCommand{
		newAPIClient: newAPIClient,
	}
	return cmd
}

// TODO(ericsnow) Change "tag" to "label" in the help text?

var listDoc = `
This command will report on the runtime state of defined payloads.

When one or more pattern is given, Juju will limit the results to only
those payloads which match *any* of the provided patterns. Each pattern
will be checked against the following info in Juju:

- unit name
- machine id
- payload type
- payload class
- payload id
- payload tag
- payload status
`

func (c *ListCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "list-payloads",
		Args:    "[pattern ...]",
		Purpose: "display status information about known payloads",
		Doc:     listDoc,
	}
}

func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	defaultFormat := "tabular"
	c.out.AddFlags(f, defaultFormat, map[string]cmd.Formatter{
		"tabular": FormatTabular,
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
	})
}

func (c *ListCommand) Init(args []string) error {
	c.patterns = args
	return nil
}

// TODO(ericsnow) Move this to a common place, like cmd/modelcmd?
const connectionError = `Unable to connect to model %q.
Please check your credentials or use 'juju bootstrap' to create a new model.

Error details:
%v
`

func (c *ListCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.newAPIClient(c)
	if err != nil {
		return fmt.Errorf(connectionError, c.ConnectionName(), err)
	}
	defer apiclient.Close()

	payloads, err := apiclient.ListFull(c.patterns...)
	if err != nil {
		if payloads == nil {
			// List call completely failed; there is nothing to report.
			return errors.Trace(err)
		}
		// Display any error, but continue to print info if some was returned.
		fmt.Fprintf(ctx.Stderr, "%v\n", err)
	}

	// Note that we do not worry about c.CompatVersion for list-payloads...
	formatter := newListFormatter(payloads)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
