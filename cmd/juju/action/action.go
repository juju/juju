// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
)

var actionDoc = `
"juju action" executes and manages actions on units; it queues up new actions,
monitors the status of running actions, and retrieves the results of completed
actions.
`

var actionPurpose = "execute, manage, monitor, and retrieve results of actions"

// NewSuperCommand returns a new action super-command.
func NewSuperCommand() cmd.Command {
	actionCmd := cmd.NewSuperCommand(
		cmd.SuperCommandParams{
			Name:        "action",
			Doc:         actionDoc,
			UsagePrefix: "juju",
			Purpose:     actionPurpose,
		})
	actionCmd.Register(envcmd.Wrap(&DefinedCommand{}))
	actionCmd.Register(envcmd.Wrap(&DoCommand{}))
	actionCmd.Register(envcmd.Wrap(&FetchCommand{}))
	actionCmd.Register(envcmd.Wrap(&StatusCommand{}))
	return actionCmd
}

// type APIClient represents the action API functionality.
type APIClient interface {
	io.Closer

	// Enqueue takes a list of Actions and queues them up to be executed by
	// the designated ActionReceiver, returning the params.Action for each
	// queued Action, or an error if there was a problem queueing up the
	// Action.
	Enqueue(params.Actions) (params.ActionResults, error)

	// ListAll takes a list of Tags representing ActionReceivers and returns
	// all of the Actions that have been queued or run by each of those
	// Entities.
	ListAll(params.Entities) (params.ActionsByReceivers, error)

	// ListPending takes a list of Tags representing ActionReceivers
	// and returns all of the Actions that are queued for each of those
	// Entities.
	ListPending(params.Entities) (params.ActionsByReceivers, error)

	// ListCompleted takes a list of Tags representing ActionReceivers
	// and returns all of the Actions that have been run on each of those
	// Entities.
	ListCompleted(params.Entities) (params.ActionsByReceivers, error)

	// Cancel attempts to cancel a queued up Action from running.
	Cancel(params.Actions) (params.ActionResults, error)

	// ServiceCharmActions is a single query which uses ServicesCharmActions to
	// get the charm.Actions for a single Service by tag.
	ServiceCharmActions(params.Entity) (*charm.Actions, error)

	// Actions fetches actions by tag.  These Actions can be used to get
	// the ActionReceiver if necessary.
	Actions(params.Entities) (params.ActionResults, error)

	// FindActionTagsByPrefix takes a list of string prefixes and finds
	// corresponding ActionTags that match that prefix.
	FindActionTagsByPrefix(params.FindTags) (params.FindTagsResults, error)
}

// ActionCommandBase is the base type for action sub-commands.
type ActionCommandBase struct {
	envcmd.EnvCommandBase
}

// NewActionAPIClient returns a client for the action api endpoint.
func (c *ActionCommandBase) NewActionAPIClient() (APIClient, error) {
	return newAPIClient(c)
}

var newAPIClient = func(c *ActionCommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return action.NewClient(root), nil
}
