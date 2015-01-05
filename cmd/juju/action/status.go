// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"launchpad.net/gnuflag"

	"github.com/juju/juju/apiserver/params"
)

// StatusCommand shows the status of an Action by ID.
type StatusCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedId string
}

const statusDoc = `
Show the status of an Action by its identifier.
`

// Set up the YAML output.
func (c *StatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "yaml", map[string]cmd.Formatter{
		"yaml": cmd.FormatYaml,
	})
}

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "<action identifier>",
		Purpose: "WIP: show results of an action by identifier",
		Doc:     statusDoc,
	}
}

func (c *StatusCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		return errors.New("no action identifier specified")
	case 1:
		c.requestedId = args[0]
		return nil
	default:
		return cmd.CheckEmpty(args[1:])
	}
}

func (c *StatusCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	actionTag, err := getActionTagFromPrefix(api, c.requestedId)
	if err != nil {
		return err
	}

	actions, err := api.Actions(params.Entities{
		Entities: []params.Entity{{actionTag.String()}},
	})
	if err != nil {
		return err
	}
	actionResults := actions.Results
	numActionResults := len(actionResults)
	if numActionResults == 0 {
		return errors.Errorf("identifier %q matched action %q, but found no results", c.requestedId, actionTag.Id())
	}
	if numActionResults != 1 {
		return errors.Errorf("too many results for action %s", actionTag.Id())
	}

	result := actionResults[0]
	if result.Error != nil {
		return result.Error
	}
	return c.out.Write(ctx, struct {
		Id     string
		Status string
	}{
		Id:     actionTag.Id(),
		Status: result.Status,
	})
}
