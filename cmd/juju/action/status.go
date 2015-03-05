// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/names"
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
Show the status of Actions matching given ID, partial ID prefix, or all Actions if no ID is supplied.
`

// Set up the output.
func (c *StatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.out.AddFlags(f, "smart", cmd.DefaultFormatters)
}

func (c *StatusCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "status",
		Args:    "[<action ID>|<action ID prefix>]",
		Purpose: "show results of all actions filtered by optional ID prefix",
		Doc:     statusDoc,
	}
}

func (c *StatusCommand) Init(args []string) error {
	switch len(args) {
	case 0:
		c.requestedId = ""
		return nil
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

	actionTags, err := getActionTagsByPrefix(api, c.requestedId)
	if err != nil {
		return err
	}

	if len(actionTags) < 1 {
		if len(c.requestedId) == 0 {
			return errors.Errorf("no actions found")
		} else {
			return errors.Errorf("no actions found matching prefix %q", c.requestedId)
		}
	}

	entities := []params.Entity{}
	for _, tag := range actionTags {
		entities = append(entities, params.Entity{tag.String()})
	}

	actions, err := api.Actions(params.Entities{Entities: entities})
	if err != nil {
		return err
	}

	if len(actions.Results) < 1 {
		return errors.Errorf("identifier %q matched action(s) %v, but found no results", c.requestedId, actionTags)
	}

	return c.out.Write(ctx, resultsToMap(actions.Results))
}

func resultsToMap(results []params.ActionResult) map[string]interface{} {
	items := []map[string]interface{}{}
	for _, item := range results {
		items = append(items, resultToMap(item))
	}
	return map[string]interface{}{"actions": items}
}

func resultToMap(result params.ActionResult) map[string]interface{} {
	item := map[string]interface{}{}
	if result.Error != nil {
		item["error"] = result.Error.Error()
	}
	if result.Action != nil {
		atag, err := names.ParseActionTag(result.Action.Tag)
		if err != nil {
			item["id"] = result.Action.Tag
		} else {
			item["id"] = atag.Id()
		}

		rtag, err := names.ParseUnitTag(result.Action.Receiver)
		if err != nil {
			item["unit"] = result.Action.Receiver
		} else {
			item["unit"] = rtag.Id()
		}

	}
	item["status"] = result.Status
	return item
}
