// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client allows access to the annotations API end point.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient creates a new client for accessing the annotations API.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "Annotations")
	return &Client{ClientFacade: frontend, facade: backend}
}

// Get returns annotations that have been set on the given entities.
func (c *Client) Get(tags []string) ([]params.AnnotationsGetResult, error) {
	annotations := params.AnnotationsGetResults{}
	if err := c.facade.FacadeCall("Get", params.AnnotationsGet{EntityTags: tags}, &annotations); err != nil {
		return annotations.Results, errors.Trace(err)
	}
	return annotations.Results, nil
}

// Set sets entity annotation pairs.
func (c *Client) Set(annotations map[string]map[string]string) error {
	args := params.AnnotationsSet{entitiesAnnotations(annotations)}
	if err := c.facade.FacadeCall("Set", args, nil); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func entitiesAnnotations(annotations map[string]map[string]string) []params.EntityAnnotations {
	all := []params.EntityAnnotations{}
	for tag, pairs := range annotations {
		one := params.EntityAnnotations{
			EntityTag:   tag,
			Annotations: pairs,
		}
		all = append(all, one)
	}
	return all
}
