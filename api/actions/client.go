// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v4"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the actions facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new actions client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Actions")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Enqueue takes a list of Actions and queues them up to be executed by
// the designated ActionReceiver, returning the params.Action for each
// queued Action, or an error if there was a problem queueing up the
// Action.
func (c *Client) Enqueue(arg params.Actions) (params.ActionResults, error) {
	results := params.ActionResults{}
	err := c.facade.FacadeCall("Enqueue", arg, &results)
	return results, err
}

// ListAll takes a list of Tags representing ActionReceivers and returns
// all of the Actions that have been queued or run by each of those
// Entities.
func (c *Client) ListAll(arg params.Tags) (params.ActionsByReceivers, error) {
	results := params.ActionsByReceivers{}
	err := c.facade.FacadeCall("ListAll", arg, &results)
	return results, err
}

// ListPending takes a list of Tags representing ActionReceivers
// and returns all of the Actions that are queued for each of those
// Entities.
func (c *Client) ListPending(arg params.Tags) (params.ActionsByReceivers, error) {
	results := params.ActionsByReceivers{}
	err := c.facade.FacadeCall("ListPending", arg, &results)
	return results, err
}

// ListCompleted takes a list of Tags representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (c *Client) ListCompleted(arg params.Tags) (params.ActionsByReceivers, error) {
	results := params.ActionsByReceivers{}
	err := c.facade.FacadeCall("ListCompleted", arg, &results)
	return results, err
}

// Cancel attempts to cancel a queued up Action from running.
func (c *Client) Cancel(arg params.Actions) (params.ActionResults, error) {
	results := params.ActionResults{}
	err := c.facade.FacadeCall("Cancel", arg, &results)
	return results, err
}

// ServicesCharmActions is a batched query for the charm.Actions for a slice
// of services by tag.
func (c *Client) ServicesCharmActions(arg params.ServiceTags) (params.ServicesCharmActionsResults, error) {
	results := params.ServicesCharmActionsResults{}
	err := c.facade.FacadeCall("ServicesCharmActions", arg, &results)
	return results, err
}

// ServiceCharmActions is a single query which uses ServicesCharmActions to
// get the charm.Actions for a single Service by tag.
func (c *Client) ServiceCharmActions(arg names.ServiceTag) (*charm.Actions, error) {
	none := &charm.Actions{}
	tags := params.ServiceTags{ServiceTags: []names.ServiceTag{arg}}
	results, err := c.ServicesCharmActions(tags)
	if err != nil {
		return none, err
	}
	if len(results.Results) != 1 {
		return none, errors.Errorf("%d results, expected 1", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return none, result.Error
	}
	if result.ServiceTag != arg {
		return none, errors.Errorf("action results received for wrong service %q", result.ServiceTag)
	}
	return result.Actions, nil
}
