// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actions

import (
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

// Enqueue takes a list of Actions and queues them up to be executed by the designated Unit.
func (c *Client) Enqueue(arg params.Actions) (params.ErrorResults, error) {
	// TODO(jcw4) implement this fully
	results := params.ErrorResults{}
	err := c.facade.FacadeCall("Enqueue", arg, &results)
	return results, err
}

// ListAll takes a list of Entities and returns all of the Actions that have been queued
// or run by that Entity.
func (c *Client) ListAll(arg params.Entities) (params.Actions, error) {
	// TODO(jcw4) implement this fully
	results := params.Actions{}
	err := c.facade.FacadeCall("ListAll", arg, &results)
	return results, err
}

// ListPending takes a list of Entities and returns all of the Actions that are queued for
// that Entity.
func (c *Client) ListPending(arg params.Entities) (params.Actions, error) {
	// TODO(jcw4) implement this fully
	results := params.Actions{}
	err := c.facade.FacadeCall("ListPending", arg, &results)
	return results, err
}

// ListCompleted takes a list of Entities and returns all of the Actions that have been
// run on that Entity.
func (c *Client) ListCompleted(arg params.Entities) (params.Actions, error) {
	// TODO(jcw4) implement this fully
	results := params.Actions{}
	err := c.facade.FacadeCall("ListCompleted", arg, &results)
	return results, err
}

// Cancel attempts to cancel a queued up Action from running.
func (c *Client) Cancel(arg params.ActionsRequest) (params.ErrorResults, error) {
	// TODO(jcw4) implement this fully
	results := params.ErrorResults{}
	err := c.facade.FacadeCall("Cancel", arg, &results)
	return results, err
}
