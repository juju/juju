// Copyright 2014-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/feature"
)

func NewCancelCommand() cmd.Command {
	return modelcmd.Wrap(&cancelCommand{})
}

type cancelCommand struct {
	ActionCommandBase
	out          cmd.Output
	requestedIDs []string
}

// Set up the output.
func (c *cancelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

const cancelDoc = `
Cancel pending or running tasks matching given IDs or partial ID prefixes.`

func (c *cancelCommand) Info() *cmd.Info {
	var info *cmd.Info
	if featureflag.Enabled(feature.ActionsV2) {
		info = &cmd.Info{
			Name:    "cancel-task",
			Args:    "(<task-id>|<task-id-prefix>) [...]",
			Purpose: "Cancel pending or running tasks.",
			Doc:     cancelDoc,
		}
	} else {
		info = &cmd.Info{
			Name:    "cancel-action",
			Args:    "(<action-id>|<action-id-prefix>) [...]",
			Purpose: "Cancel pending or running actions.",
			Doc:     strings.Replace(cancelDoc, "task", "action", -1),
			Aliases: []string{"cancel-task"},
		}
	}
	return jujucmd.Info(info)
}

func (c *cancelCommand) Init(args []string) error {
	c.requestedIDs = args
	return nil
}

func (c *cancelCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	if len(c.requestedIDs) == 0 {
		return errors.Errorf("no actions specified")
	}

	var actionTags []names.ActionTag
	for _, requestedID := range c.requestedIDs {
		requestedActionTags, err := getActionTagsByPrefix(api, requestedID)
		if err != nil {
			return err
		}

		// If a non existing ID was submitted we abort the command taking no further action.
		if len(requestedActionTags) < 1 {
			return errors.Errorf("no actions found matching prefix %s, no actions have been canceled", requestedID)
		}

		actionTags = append(actionTags, requestedActionTags...)
	}

	entities := []params.Entity{}
	for _, tag := range actionTags {
		entities = append(entities, params.Entity{Tag: tag.String()})
	}

	actions, err := api.Cancel(params.Entities{Entities: entities})
	if err != nil {
		return err
	}

	if len(actions.Results) < 1 {
		return errors.Errorf("identifier(s) %q matched action(s) %q, but no actions were canceled", c.requestedIDs, actionTags)
	}

	type unCanceledAction struct {
		ActionTag names.ActionTag
		Result    *params.ActionResult
	}
	var unCanceledActions []unCanceledAction
	var canceledActions []params.ActionResult

	for i, result := range actions.Results {
		if result.Action != nil {
			canceledActions = append(canceledActions, result)
		} else {
			unCanceledActions = append(unCanceledActions, unCanceledAction{actionTags[i], &result})
		}
	}

	if len(canceledActions) > 0 {
		err = c.out.Write(ctx, resultsToMap(canceledActions))
	}

	if len(unCanceledActions) > 0 {
		message := ("The following actions could not be canceled:\n")
		for _, a := range unCanceledActions {
			message += fmt.Sprintf("action: %s, error: %s\n", a.ActionTag, a.Result.Message)
		}

		logger.Warningf(message)
	}

	return err
}
