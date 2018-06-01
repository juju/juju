// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/naturalsort"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
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
	c.ActionCommandBase.SetFlags(f)

	// `listCommand's` default format depends on the value of another flag.
	// That is, if no default is selected using the `--format` flag, then
	// the output format depends on whether or not the user has specified
	// the `--schema` flag (schema output does not support tabular which is
	// the default for all other output from this command). Currently the
	// `cmd` package's `Output` structure has no methods that support
	// selecting a default format dynamically. Here we introduce a "default"
	// default which serves to indicate that the user wants default
	// formatting behavior. This allows us to select the appropriate default
	// behavior in the presence of the "default" format value.
	c.out.AddFlags(f, "default", map[string]cmd.Formatter{
		"yaml":    cmd.FormatYaml,
		"json":    cmd.FormatJson,
		"tabular": c.printTabular,
		"default": c.dummyDefault,
	})
	f.BoolVar(&c.fullSchema, "schema", false, "Display the full action schema")
}

func (c *listCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "actions",
		Args:    "<application name>",
		Purpose: "List actions defined for an application.",
		Doc:     listDoc,
		Aliases: []string{"list-actions"},
	}
}

// Init validates the application name and any other options.
func (c *listCommand) Init(args []string) error {
	if c.out.Name() == "tabular" && c.fullSchema {
		return errors.New("full schema not compatible with tabular output")
	}
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

	actions, err := api.ApplicationCharmActions(params.Entity{Tag: c.applicationTag.String()})
	if err != nil {
		return err
	}

	if c.fullSchema {
		verboseSpecs := make(map[string]interface{})
		for k, v := range actions {
			verboseSpecs[k] = v.Params
		}

		if c.out.Name() == "default" {
			return c.out.WriteFormatter(ctx, cmd.FormatYaml, verboseSpecs)
		} else {
			return c.out.Write(ctx, verboseSpecs)
		}
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
	naturalsort.Sort(sortedNames)

	var output interface{}
	switch c.out.Name() {
	case "yaml", "json":
		output = shortOutput
	default:
		if len(sortedNames) == 0 {
			ctx.Infof("No actions defined for %s.", c.applicationTag.Id())
			return nil
		}
		var list []listOutput
		for _, name := range sortedNames {
			list = append(list, listOutput{name, shortOutput[name]})
		}
		output = list
	}

	if c.out.Name() == "default" {
		return c.out.WriteFormatter(ctx, c.printTabular, output)
	} else {
		return c.out.Write(ctx, output)
	}

}

type listOutput struct {
	action      string
	description string
}

// printTabular prints the list of actions in tabular format
func (c *listCommand) printTabular(writer io.Writer, value interface{}) error {
	list, ok := value.([]listOutput)
	if !ok {
		return errors.New("unexpected value")
	}

	tw := output.TabWriter(writer)
	fmt.Fprintf(tw, "%s\t%s\n", "Action", "Description")
	for _, value := range list {
		fmt.Fprintf(tw, "%s\t%s\n", value.action, strings.TrimSpace(value.description))
	}
	tw.Flush()
	return nil
}

// This method represents a default format that is used to express the need for
// a dynamically selected default. That is, when the `actions` command
// determines its default output format based on the presence of a flag other
// than "format", then this method is used to indicate that a "dynamic" default
// is desired. NOTE: It is very possible that this functionality should live in
// the cmd package where this kind of thing can be handled in a more elegant
// and DRY fashion.
func (c *listCommand) dummyDefault(writer io.Writer, value interface{}) error {
	return nil
}
