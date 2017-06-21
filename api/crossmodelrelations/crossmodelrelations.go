// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelrelations

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
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

// PublishRelationChange publishes relation changes to the
// model hosting the remote application involved in the relation.
func (c *Client) PublishRelationChange(change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("PublishRelationChange", args, &results)
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

// RegisterRemoteRelation sets up the remote model to participate
// in the specified relations.
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

// WatchRelationUnits returns a watcher that notifies of changes to the
// units in the remote model for the relation with the given remote id.
func (c *Client) WatchRelationUnits(remoteRelationId params.RemoteEntityId) (watcher.RelationUnitsWatcher, error) {
	args := params.RemoteEntities{Entities: []params.RemoteEntityId{remoteRelationId}}
	var results params.RelationUnitsWatchResults
	err := c.facade.FacadeCall("WatchRelationUnits", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewRelationUnitsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the remote model.
func (c *Client) RelationUnitSettings(relationUnits []params.RemoteRelationUnit) ([]params.SettingsResult, error) {
	args := params.RemoteRelationUnits{relationUnits}
	var results params.SettingsResults
	err := c.facade.FacadeCall("RelationUnitSettings", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(relationUnits) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(relationUnits), len(results.Results))
	}
	return results.Results, nil
}
