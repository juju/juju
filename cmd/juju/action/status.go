// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"launchpad.net/gnuflag"
)

// StatusCommand shows the status of an Action by ID.
type StatusCommand struct {
	ActionCommandBase
	requestedId string
	out         cmd.Output
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

	tags, err := api.FindActionTagsByPrefix(params.FindTags{Prefixes: []string{c.requestedId}})
	if err != nil {
		return err
	}

	results, ok := tags.Matches[c.requestedId]
	if !ok || len(results) < 1 {
		return errors.Errorf("actions for identifier %q not found", c.requestedId)
	}

	actiontags, rejects := getActionTags(results)
	if len(rejects) > 0 {
		return errors.Errorf("identifier %q got unrecognized entity tags %v", c.requestedId, rejects)
	}

	if len(actiontags) > 1 {
		return errors.Errorf("identifier %q matched multiple actions %v", c.requestedId, actiontags)
	}

	actionTag := actiontags[0]

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
