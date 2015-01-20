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
	if err := c.facade.FacadeCall("Get", entitiesFromTags(tags), &annotations); err != nil {
		return annotations.Results, errors.Trace(err)
	}
	return annotations.Results, nil
}

// Set sets entity annotation pairs.
func (c *Client) Set(annotations map[string]map[string]string) ([]params.ErrorResult, error) {
	args := params.AnnotationsSet{entitiesAnnotations(annotations)}
	results := new(params.ErrorResults)
	if err := c.facade.FacadeCall("Set", args, results); err != nil {
		return nil, errors.Trace(err)
	}
	return results.Results, nil
}

func entitiesFromTags(tags []string) params.Entities {
	entities := []params.Entity{}
	for _, tag := range tags {
		entities = append(entities, params.Entity{tag})
	}
	return params.Entities{entities}
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
