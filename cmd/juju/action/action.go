// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"io"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher"
)

// APIClient represents the action API functionality.
type APIClient interface {
	io.Closer

	// RunOnAllMachines runs the command on all the machines with the specified
	// timeout.
	RunOnAllMachines(commands string, timeout time.Duration) (action.EnqueuedActions, error)

	// Run the Commands specified on the machines identified through the ids
	// provided in the machines, applications and units slices.
	Run(action.RunParams) (action.EnqueuedActions, error)

	// EnqueueOperation takes a list of Actions and queues them up to be executed as
	// an operation, each action running as a task on the the designated ActionReceiver.
	// We return the ID of the overall operation and each individual task.
	EnqueueOperation([]action.Action) (action.EnqueuedActions, error)

	// Cancel attempts to cancel a queued up Action from running.
	Cancel([]string) ([]action.ActionResult, error)

	// ApplicationCharmActions is a single query which uses ApplicationsCharmsActions to
	// get the charm.Actions for a single application by tag.
	ApplicationCharmActions(appName string) (map[string]action.ActionSpec, error)

	// Actions fetches actions by tag.  These Actions can be used to get
	// the ActionReceiver if necessary.
	Actions([]string) ([]action.ActionResult, error)

	// ListOperations fetches the operation summaries for specified apps/units.
	ListOperations(action.OperationQueryArgs) (action.Operations, error)

	// Operation fetches the operation with the specified id.
	Operation(id string) (action.Operation, error)

	// WatchActionProgress reports on logged action progress messages.
	WatchActionProgress(actionId string) (watcher.StringsWatcher, error)
}

// ActionCommandBase is the base type for action sub-commands.
type ActionCommandBase struct {
	modelcmd.ModelCommandBase
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
