// Copyright 2014-2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/cmd"
	errors "github.com/juju/errors"
	"github.com/juju/gnuflag"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
)

func NewCancelCommand() cmd.Command {
	return modelcmd.Wrap(&cancelCommand{})
}

type cancelCommand struct {
	ActionCommandBase
	out          cmd.Output
	requestedIds []string
}

// Set up the output.
func (c *cancelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ActionCommandBase.SetFlags(f)
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

const cancelDoc = `
Cancel actions matching given IDs or partial ID prefixes.`

func (c *cancelCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "cancel-action",
		Args:    "<<action ID | action ID prefix>...>",
		Purpose: "Cancel pending actions.",
		Doc:     cancelDoc,
	}
}

func (c *cancelCommand) Init(args []string) error {
	c.requestedIds = args
	return nil
}

func (c *cancelCommand) Run(ctx *cmd.Context) error {
	api, err := c.NewActionAPIClient()
	if err != nil {
		return err
	}
	defer api.Close()

	if len(c.requestedIds) == 0 {
		return errors.Errorf("no actions specified")
	}

	var actionTags []names.ActionTag
	for _, requestedId := range c.requestedIds {
		requestedActionTags, err := getActionTagsByPrefix(api, requestedId)
		if err != nil {
			return err
		}

		// If a non existing ID was submitted we abort the command taking no further action.
		if len(requestedActionTags) < 1 {
			return errors.Errorf("no actions found matching prefix %s, no actions have been canceled", requestedId)
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
		return errors.Errorf("identifier(s) %q matched action(s) %q, but no actions were canceled", c.requestedIds, actionTags)
	}

	var unCanceledActions []string
	var canceledActions []params.ActionResult
	for _, result := range actions.Results {
		if result.Error != nil {
			unCanceledActions = append(unCanceledActions, result.Action.Tag)
			continue
		}
		canceledActions = append(canceledActions, result)
	}

	if len(canceledActions) > 0 {
		err = c.out.Write(ctx, resultsToMap(canceledActions))
	}

	if len(unCanceledActions) > 0 {
		logger.Warningf("The following actions could not be canceled: %v. The actions may not have been in the pending state at the time of attempted cancellation", unCanceledActions)
	}

	return err
}
