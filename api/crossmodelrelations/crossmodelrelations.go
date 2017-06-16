// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the crossmodelrelations api facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client-side CrossModelRelations facade.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, "CrossModelRelations")
	return &Client{facadeCaller}
}

// PublishLocalRelationChange publishes local relations changes to the
// remote side offering those relations.
func (c *Client) PublishLocalRelationChange(change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("PublishLocalRelationChange", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return errors.Trace(result.Error)
	}
	return nil
}

// RegisterRemoteRelations sets up the local model to participate in the specified relations.
func (c *Client) RegisterRemoteRelations(relations ...params.RegisterRemoteRelation) ([]params.RemoteEntityIdResult, error) {
	args := params.RegisterRemoteRelations{Relations: relations}
	var results params.RemoteEntityIdResults
	err := c.facade.FacadeCall("RegisterRemoteRelations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(relations) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(relations), len(results.Results))
	}
	return results.Results, nil
}
