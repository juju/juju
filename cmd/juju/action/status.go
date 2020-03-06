// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

func NewStatusCommand() cmd.Command {
	return modelcmd.Wrap(&statusCommand{})
}

// statusCommand shows the status of an Action by ID.
type statusCommand struct {
	ActionCommandBase
	out         cmd.Output
	requestedId string
	name        string
}

const statusDoc = `
Show the status of Actions matching given ID, partial ID prefix, or all Actions if no ID is supplied.
If --name <name> is provided the search will be done by name rather than by ID.
`

// Set up the output.
func (c *statusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
	f.StringVar(&c.name, "name", "", "Action name")
}

func (c *statusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "show-action-status",
		Args:    "[<action>|<action-id-prefix>]",
		Purpose: "Show results of all actions filtered by optional ID prefix.",
		Doc:     statusDoc,
	})
}

func (c *statusCommand) Init(args []string) error {
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

func (c *statusCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	if c.name != "" {
		actions, err := GetActionsByName(api, c.name)
		if err != nil {
			return errors.Trace(err)
		}
		return c.out.Write(ctx, resultsToMap(actions))
	}

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
		entities = append(entities, params.Entity{Tag: tag.String()})
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

// resultsToMap is a helper function that takes in a []params.ActionResult
// and returns a map[string]interface{} ready to be served to the
// formatter for printing.
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
		item["action"] = result.Action.Name
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

	// result.Completed uses the zero-value to indicate not completed
	if result.Completed.Equal(time.Time{}) {
		item["completed at"] = "n/a"
	} else {
		item["completed at"] = result.Completed.UTC().Format("2006-01-02 15:04:05")
	}

	return item
}

// GetActionsByName takes an action APIClient and a name and returns a list of
// ActionResults.
func GetActionsByName(api APIClient, name string) ([]params.ActionResult, error) {
	nothing := []params.ActionResult{}
	results, err := api.FindActionsByNames(params.FindActionsByNames{ActionNames: []string{name}})
	if err != nil {
		return nothing, errors.Trace(err)
	}
	if len(results.Actions) != 1 {
		return nothing, errors.Errorf("expected one result got %d", len(results.Actions))
	}
	result := results.Actions[0]
	if result.Error != nil {
		return nothing, result.Error
	}
	if len(result.Actions) < 1 {
		return nothing, errors.Errorf("no actions were found for name %s", name)
	}
	return result.Actions, nil

}
