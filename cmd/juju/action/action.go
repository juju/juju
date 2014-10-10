// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"io"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/juju/api/action"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"gopkg.in/juju/charm.v4"
)

// FeatureFlag is the name of the feature for the JUJU_DEV_FEATURE_FLAGS
// envar.  Add this string to the envar to enable this action command.
const FeatureFlag string = "action"

var actionDoc = `
"juju action" executes and manages actions on units, monitors their status,
and retrieves their results.
`

var actionPurpose = "execute, manage, monitor, and retrieve results of actions"

// Command is the top-level command wrapping all action functionality.
type ActionCommand struct {
	cmd.SuperCommand
}

// NewActionCommand returns a new action super-command.
func NewActionCommand() cmd.Command {
	actionCmd := ActionCommand{
		SuperCommand: *cmd.NewSuperCommand(
			cmd.SuperCommandParams{
				Name:        "action",
				Doc:         actionDoc,
				UsagePrefix: "juju",
				Purpose:     actionPurpose,
			},
		),
	}
	actionCmd.Register(envcmd.Wrap(&DefinedCommand{}))
	actionCmd.Register(envcmd.Wrap(&DoCommand{}))
	actionCmd.Register(envcmd.Wrap(&WaitCommand{}))
	actionCmd.Register(envcmd.Wrap(&QueueCommand{}))
	actionCmd.Register(envcmd.Wrap(&KillCommand{}))
	actionCmd.Register(envcmd.Wrap(&StatusCommand{}))
	actionCmd.Register(envcmd.Wrap(&LogCommand{}))
	actionCmd.Register(envcmd.Wrap(&FetchCommand{}))
	return &actionCmd
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
