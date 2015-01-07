// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the annotations API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
	st     *api.State
}

// NewClient creates a new client for accessing the annotations API.
func NewClient(st *api.State) *Client {
	frontend, backend := base.NewClientFacade(st, "Annotations")
	return &Client{ClientFacade: frontend, st: st, facade: backend}
}

// Get returns annotations that have been set on the given entities.
func (c *Client) Get(args params.Entities) (params.AnnotationsGetResults, error) {
	annotations := params.AnnotationsGetResults{}
	if err := c.facade.FacadeCall("Get", args, &annotations); err != nil {
		return annotations, errors.Trace(err)
	} else {
		return annotations, nil
	}
}

// Set sets the same annotation pairs on all given entities.
func (c *Client) Set(entities params.Entities, pairs map[string]string) error {
	args := params.AnnotationsSet{entities, pairs}
	if err := c.facade.FacadeCall("Set", args, nil); err != nil {
		return errors.Trace(err)
	}
	return nil

}
