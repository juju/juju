// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package payload

import (
	"fmt"
	"io"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/payloads"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	corepayloads "github.com/juju/juju/core/payloads"
)

// ListAPI has the API methods needed by ListCommand.
type ListAPI interface {
	ListFull(patterns ...string) ([]corepayloads.FullPayloadInfo, error)
	io.Closer
}

// ListCommand implements the payloads command.
type ListCommand struct {
	modelcmd.ModelCommandBase
	out      cmd.Output
	patterns []string

	newAPIClient func() (ListAPI, error)
}

// NewListCommand returns a new command that lists charm payloads
// in the current environment.
func NewListCommand() modelcmd.ModelCommand {
	c := &ListCommand{}
	c.newAPIClient = func() (ListAPI, error) {
		apiRoot, err := c.NewAPIRoot()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return payloads.NewClient(apiRoot), nil
	}
	return modelcmd.Wrap(c)
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
	return jujucmd.Info(&cmd.Info{
		Name:    "payloads",
		Args:    "[pattern ...]",
		Purpose: "Display status information about known payloads.",
		Doc:     listDoc,
		Aliases: []string{"list-payloads"},
	})
}

func (c *ListCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
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

func (c *ListCommand) Run(ctx *cmd.Context) error {
	apiclient, err := c.newAPIClient()
	if err != nil {
		return errors.Trace(err)
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

	if len(payloads) == 0 {
		ctx.Infof("No payloads to display.")
		return nil
	}

	// Note that we do not worry about c.CompatVersion for payloads...
	formatter := newListFormatter(payloads)
	formatted := formatter.format()
	return c.out.Write(ctx, formatted)
}
