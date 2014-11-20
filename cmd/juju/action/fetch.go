// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"

	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/names"
	"launchpad.net/gnuflag"
)

// FetchCommand fetches the results of an action by UUID.
type FetchCommand struct {
	ActionCommandBase
	actionTag  names.ActionTag
	fullSchema bool
	out        cmd.Output
}

const fetchDoc = `
Show the results returned by an action.
`

// Set up the YAML output.
func (c *FetchCommand) SetFlags(f *gnuflag.FlagSet) {
	// TODO(binary132) add json output?
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

func (c *FetchCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "fetch",
		Args:    "<action UUID>",
		Purpose: "WIP: show results of an action by UUID",
		Doc:     fetchDoc,
	}
}

// Init validates the action ID and any other options.
func (c *FetchCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no action UUID specified")
	case 1:
		if !names.IsValidAction(args[0]) {
			return errors.Errorf("invalid action ID %q", args[0])
		}
		c.actionTag = names.NewActionTag(args[0])
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

// Run issues the API call to get Actions by UUID.
func (c *FetchCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actions, err := api.Actions(params.Entities{
		Entities: []params.Entity{{c.actionTag.String()}},
	})
	if err != nil {
		return err
	}
	actionResults := actions.Results
	numActionResults := len(actionResults)
	if numActionResults == 0 {
		return c.out.Write(ctx, fmt.Sprintf("No results for action %s", c.actionTag.Id()))
	}
	if numActionResults != 1 {
		return errors.Errorf("too many results for action %s", c.actionTag.Id())
	}

	result := actionResults[0]
	if result.Error != nil {
		return result.Error
	}
	return c.out.Write(ctx, formatActionResult(result))
}

func formatActionResult(result params.ActionResult) map[string]interface{} {
	return map[string]interface{}{
		"status":  result.Status,
		"message": result.Message,
		"results": result.Output,
	}
}
