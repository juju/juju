// Copyright 2014-2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	actionapi "github.com/juju/juju/v2/api/client/action"
	jujucmd "github.com/juju/juju/v2/cmd"
	"github.com/juju/juju/v2/cmd/modelcmd"
	"github.com/juju/juju/v2/cmd/output"
	"github.com/juju/juju/v2/rpc/params"
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

	var actionIds []string
	for _, tag := range actionTags {
		actionIds = append(actionIds, tag.Id())
	}

	actions, err := api.Actions(actionIds)
	if err != nil {
		return err
	}

	if len(actions) < 1 {
		return errors.Errorf("identifier %q matched action(s) %v, but found no results", c.requestedId, actionTags)
	}

	return c.out.Write(ctx, resultsToMap(actions))
}

// resultsToMap is a helper function that takes in a []params.ActionResult
// and returns a map[string]interface{} ready to be served to the
// formatter for printing.
func resultsToMap(results []actionapi.ActionResult) map[string]interface{} {
	items := []map[string]interface{}{}
	for _, item := range results {
		items = append(items, resultToMap(item))
	}
	return map[string]interface{}{"actions": items}
}

func resultToMap(result actionapi.ActionResult) map[string]interface{} {
	item := map[string]interface{}{}
	if result.Error != nil {
		item["error"] = result.Error.Error()
	}
	if result.Action != nil {
		item["action"] = result.Action.Name
		item["id"] = result.Action.ID

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
func GetActionsByName(api APIClient, name string) ([]actionapi.ActionResult, error) {
	nothing := []actionapi.ActionResult{}
	results, err := api.FindActionsByNames(params.FindActionsByNames{ActionNames: []string{name}})
	if err != nil {
		return nothing, errors.Trace(err)
	}
	if len(results) != 1 {
		return nothing, errors.Errorf("expected one result got %d", len(results))
	}
	actions := results[name]
	if len(actions) < 1 {
		return nothing, errors.Errorf("no actions were found for name %s", name)
	}
	result := actions[0]
	if result.Error != nil {
		return nothing, result.Error
	}
	return actions, nil

}
