// Copyright 2014, 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"io"
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/api/client/action"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/watcher"
)

// APIClient represents the action API functionality.
type APIClient interface {
	io.Closer

	// RunOnAllMachines runs the command on all the machines with the specified
	// timeout.
	RunOnAllMachines(ctx context.Context, commands string, timeout time.Duration) (action.EnqueuedActions, error)

	// Run the Commands specified on the machines identified through the ids
	// provided in the machines, applications and units slices.
	Run(context.Context, action.RunParams) (action.EnqueuedActions, error)

	// EnqueueOperation takes a list of Actions and queues them up to be executed as
	// an operation, each action running as a task on the the designated ActionReceiver.
	// We return the ID of the overall operation and each individual task.
	EnqueueOperation(context.Context, []action.Action) (action.EnqueuedActions, error)

	// Cancel attempts to cancel a queued up Action from running.
	Cancel(context.Context, []string) ([]action.ActionResult, error)

	// ApplicationCharmActions is a single query which uses ApplicationsCharmsActions to
	// get the charm.Actions for a single application by tag.
	ApplicationCharmActions(ctx context.Context, appName string) (map[string]action.ActionSpec, error)

	// Actions fetches actions by tag.  These Actions can be used to get
	// the ActionReceiver if necessary.
	Actions(context.Context, []string) ([]action.ActionResult, error)

	// ListOperations fetches the operation summaries for specified apps/units.
	ListOperations(context.Context, action.OperationQueryArgs) (action.Operations, error)

	// Operation fetches the operation with the specified id.
	Operation(ctx context.Context, id string) (action.Operation, error)

	// WatchActionProgress reports on logged action progress messages.
	WatchActionProgress(ctx context.Context, actionId string) (watcher.StringsWatcher, error)
}

// ActionCommandBase is the base type for action sub-commands.
type ActionCommandBase struct {
	modelcmd.ModelCommandBase
}

// NewActionAPIClient returns a client for the action api endpoint.
func (c *ActionCommandBase) NewActionAPIClient(ctx context.Context) (APIClient, error) {
	return newAPIClient(ctx, c)
}

var newAPIClient = func(ctx context.Context, c *ActionCommandBase) (APIClient, error) {
	root, err := c.NewAPIRoot(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return action.NewClient(root), nil
}
