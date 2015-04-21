// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package action

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the action facade.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new actions client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Action")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Actions takes a list of ActionTags, and returns the full
// Action for each ID.
func (c *Client) Actions(arg params.Entities) (params.ActionResults, error) {
	results := params.ActionResults{}
	err := c.facade.FacadeCall("Actions", arg, &results)
	return results, err
}

// FindActionTagsByPrefix takes a list of string prefixes and finds
// corresponding ActionTags that match that prefix.
func (c *Client) FindActionTagsByPrefix(arg params.FindTags) (params.FindTagsResults, error) {
	results := params.FindTagsResults{}
	err := c.facade.FacadeCall("FindActionTagsByPrefix", arg, &results)
	return results, err
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

// ListAll takes a list of Entities representing ActionReceivers and returns
// all of the Actions that have been queued or run by each of those
// Entities.
func (c *Client) ListAll(arg params.Entities) (params.ActionsByReceivers, error) {
	results := params.ActionsByReceivers{}
	err := c.facade.FacadeCall("ListAll", arg, &results)
	return results, err
}

// ListPending takes a list of Entities representing ActionReceivers
// and returns all of the Actions that are queued for each of those
// Entities.
func (c *Client) ListPending(arg params.Entities) (params.ActionsByReceivers, error) {
	results := params.ActionsByReceivers{}
	err := c.facade.FacadeCall("ListPending", arg, &results)
	return results, err
}

// ListCompleted takes a list of Entities representing ActionReceivers
// and returns all of the Actions that have been run on each of those
// Entities.
func (c *Client) ListCompleted(arg params.Entities) (params.ActionsByReceivers, error) {
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

// servicesCharmActions is a batched query for the charm.Actions for a slice
// of services by Entity.
func (c *Client) servicesCharmActions(arg params.Entities) (params.ServicesCharmActionsResults, error) {
	results := params.ServicesCharmActionsResults{}
	err := c.facade.FacadeCall("ServicesCharmActions", arg, &results)
	return results, err
}

// ServiceCharmActions is a single query which uses ServicesCharmActions to
// get the charm.Actions for a single Service by tag.
func (c *Client) ServiceCharmActions(arg params.Entity) (*charm.Actions, error) {
	none := &charm.Actions{}
	tags := params.Entities{Entities: []params.Entity{{Tag: arg.Tag}}}
	results, err := c.servicesCharmActions(tags)
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
	if result.ServiceTag != arg.Tag {
		return none, errors.Errorf("action results received for wrong service %q", result.ServiceTag)
	}
	return result.Actions, nil
}
