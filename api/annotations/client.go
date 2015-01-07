// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
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

// GetEntitiesAnnotations returns annotations that have been set on the given entities.
func (c *Client) GetEntitiesAnnotations(args params.Entities) (params.GetEntitiesAnnotationsResults, error) {
	annotations := params.GetEntitiesAnnotationsResults{}
	err := c.facade.FacadeCall("GetEntitiesAnnotations", args, &annotations)
	return annotations, err
}

// SetEntitiesAnnotations sets the same annotation pairs on all given entities.
func (c *Client) SetEntitiesAnnotations(entities params.Entities, pairs map[string]string) error {
	args := params.SetEntitiesAnnotations{entities, pairs}
	return c.facade.FacadeCall("SetEntitiesAnnotations", args, nil)
}
