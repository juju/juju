// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"bytes"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/utils"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
)

func NewListCommand() cmd.Command {
	return modelcmd.Wrap(&listCommand{})
}

// listCommand lists actions defined by the charm of a given service.
type listCommand struct {
	ActionCommandBase
	applicationTag names.ApplicationTag
	fullSchema     bool
	out            cmd.Output
}

const listDoc = `
List the actions available to run on the target application, with a short
description.  To show the full schema for the actions, use --schema.

For more information, see also the 'run-action' command, which executes actions.
`

// Set up the output.
func (c *listCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
	f.BoolVar(&c.fullSchema, "schema", false, "Display the full action schema")
}

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "actions",
		Args:    "<application name>",
		Purpose: "List actions defined for a service.",
		Doc:     listDoc,
		Aliases: []string{"list-actions"},
	}
}

// Init validates the service name and any other options.
func (c *listCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no application name specified")
	case 1:
		svcName := args[0]
		if !names.IsValidApplication(svcName) {
			return errors.Errorf("invalid application name %q", svcName)
		}
		c.applicationTag = names.NewApplicationTag(svcName)
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run grabs the Actions spec from the api.  It then sets up a sensible
// output format for the map.
func (c *listCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actions, err := api.ApplicationCharmActions(params.Entity{c.applicationTag.String()})
	if err != nil {
		return err
	}

	if len(actions) == 0 {
		return c.out.Write(ctx, "No actions defined for "+c.applicationTag.Id())
	}

	if c.fullSchema {
		verboseSpecs := make(map[string]interface{})
		for k, v := range actions {
			verboseSpecs[k] = v.Params
		}

		return c.out.Write(ctx, verboseSpecs)
	}

	shortOutput := make(map[string]string)
	var sortedNames []string
	for name, action := range actions {
		shortOutput[name] = action.Description
		if shortOutput[name] == "" {
			shortOutput[name] = "No description"
		}
		sortedNames = append(sortedNames, name)
	}
	utils.SortStringsNaturally(sortedNames)
	return c.printTabular(ctx, shortOutput, sortedNames)
}

// printTabular prints the list of actions in tabular format
func (c *listCommand) printTabular(ctx *cmd.Context, actions map[string]string, sortedNames []string) error {
	var out bytes.Buffer
	const (
		// To format things into columns.
		minwidth = 0
		tabwidth = 2
		padding  = 2
		padchar  = ' '
		flags    = 0
	)
	tw := tabwriter.NewWriter(&out, minwidth, tabwidth, padding, padchar, flags)
	fmt.Fprintf(tw, "%s\t%s\n", "ACTION", "DESCRIPTION")
	for _, name := range sortedNames {
		fmt.Fprintf(tw, "%s\t%s\n", name, strings.TrimSpace(actions[name]))
	}
	tw.Flush()
	return c.out.Write(ctx, string(out.Bytes()))
}
