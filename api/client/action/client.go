// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to the action facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new actions client.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "Action", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// Actions takes a list of action IDs, and returns the full
// Action for each ID.
func (c *Client) Actions(actionIDs []string) ([]ActionResult, error) {
	arg := params.Entities{Entities: make([]params.Entity, len(actionIDs))}
	for i, ID := range actionIDs {
		arg.Entities[i].Tag = names.NewActionTag(ID).String()
	}
	results := params.ActionResults{}
	err := c.facade.FacadeCall(context.TODO(), "Actions", arg, &results)
	return unmarshallActionResults(results.Results), err
}

// ListOperations fetches the operation summaries for specified apps/units.
func (c *Client) ListOperations(arg OperationQueryArgs) (Operations, error) {
	args := params.OperationQueryArgs{
		Applications: arg.Applications,
		Units:        arg.Units,
		Machines:     arg.Machines,
		ActionNames:  arg.ActionNames,
		Status:       arg.Status,
		Offset:       arg.Offset,
		Limit:        arg.Limit,
	}
	results := params.OperationResults{}
	err := c.facade.FacadeCall(context.TODO(), "ListOperations", args, &results)
	if params.ErrCode(err) == params.CodeNotFound {
		err = nil
	}
	return unmarshallOperations(results), err
}

// Operation fetches the operation with the specified ID.
func (c *Client) Operation(ID string) (Operation, error) {
	arg := params.Entities{
		Entities: []params.Entity{{names.NewOperationTag(ID).String()}},
	}
	var results params.OperationResults
	err := c.facade.FacadeCall(context.TODO(), "Operations", arg, &results)
	if err != nil {
		return Operation{}, err
	}
	if len(results.Results) != 1 {
		return Operation{}, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return Operation{}, maybeNotFound(result.Error)
	}
	return unmarshallOperation(result), nil
}

// maybeNotFound returns an error satisfying errors.IsNotFound
// if the supplied error has a CodeNotFound error.
func maybeNotFound(err *params.Error) error {
	if err == nil || !params.IsCodeNotFound(err) {
		return err
	}
	return errors.NewNotFound(err, "")
}

// EnqueueOperation takes a list of Actions and queues them up to be executed as
// an operation, each action running as a task on the the designated ActionReceiver.
// We return the ID of the overall operation and each individual task.
func (c *Client) EnqueueOperation(actions []Action) (EnqueuedActions, error) {
	arg := params.Actions{Actions: make([]params.Action, len(actions))}
	for i, a := range actions {
		arg.Actions[i] = params.Action{
			Receiver:   a.Receiver,
			Name:       a.Name,
			Parameters: a.Parameters,
		}
	}
	results := params.EnqueuedActions{}
	err := c.facade.FacadeCall(context.TODO(), "EnqueueOperation", arg, &results)
	if err != nil {
		return EnqueuedActions{}, errors.Trace(err)
	}
	return unmarshallEnqueuedActions(results)
}

// Cancel attempts to cancel a queued up Action from running.
func (c *Client) Cancel(actionIDs []string) ([]ActionResult, error) {
	arg := params.Entities{Entities: make([]params.Entity, len(actionIDs))}
	for i, ID := range actionIDs {
		arg.Entities[i].Tag = names.NewActionTag(ID).String()
	}
	results := params.ActionResults{}
	err := c.facade.FacadeCall(context.TODO(), "Cancel", arg, &results)
	return unmarshallActionResults(results.Results), err
}

// applicationsCharmActions is a batched query for the charm.Actions for a slice
// of applications by Entity.
func (c *Client) applicationsCharmActions(arg params.Entities) (params.ApplicationsCharmActionsResults, error) {
	results := params.ApplicationsCharmActionsResults{}
	err := c.facade.FacadeCall(context.TODO(), "ApplicationsCharmsActions", arg, &results)
	return results, err
}

// ApplicationCharmActions is a single query which uses ApplicationsCharmsActions to
// get the charm.Actions for a single Application by tag.
func (c *Client) ApplicationCharmActions(appName string) (map[string]ActionSpec, error) {
	tag := names.NewApplicationTag(appName)
	tags := params.Entities{Entities: []params.Entity{{Tag: tag.String()}}}
	results, err := c.applicationsCharmActions(tags)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("%d results, expected 1", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	if result.ApplicationTag != tag.String() {
		return nil, errors.Errorf("action results received for wrong application %q", result.ApplicationTag)
	}
	return unmarshallActionSpecs(result.Actions), nil
}

// WatchActionProgress returns a watcher that reports on action log messages.
// The result strings are json formatted core.actions.ActionMessage objects.
func (c *Client) WatchActionProgress(actionId string) (watcher.StringsWatcher, error) {
	var results params.StringsWatchResults
	args := params.Entities{
		Entities: []params.Entity{
			{Tag: names.NewActionTag(actionId).String()},
		},
	}
	err := c.facade.FacadeCall(context.TODO(), "WatchActionsProgress", args, &results)
	if err != nil {
		return nil, err
	}
	if len(results.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}
