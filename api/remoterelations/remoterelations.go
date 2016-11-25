// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/watcher"
)

const remoteRelationsFacade = "RemoteRelations"

// Client provides access to the remoterelations api facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client-side RemoteRelations facade.
func NewClient(caller base.APICaller) *Client {
	facadeCaller := base.NewFacadeCaller(caller, remoteRelationsFacade)
	return &Client{facadeCaller}
}

// PublishLocalRelationChange publishes local relations changes to the
// remote side offering those relations.
func (c *Client) PublishLocalRelationChange(change params.RemoteRelationsChange) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationsChange{change},
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

// ConsumeRemoteApplicationChange consumes remote changes to applications into the local model.
func (c *Client) ConsumeRemoteApplicationChange(change params.RemoteApplicationChange) error {
	args := params.RemoteApplicationChanges{
		Changes: []params.RemoteApplicationChange{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("ConsumeRemoteApplicationChange", args, &results)
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

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (c *Client) ExportEntities(tags []names.Tag) ([]params.RemoteEntityIdResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.RemoteEntityIdResults
	err := c.facade.FacadeCall("ExportEntities", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// RelationUnitSettings returns the relation unit settings for the given relation units in the local model.
func (c *Client) RelationUnitSettings(relationUnits []params.RelationUnit) ([]params.SettingsResult, error) {
	args := params.RelationUnits{relationUnits}
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

// Relations returns information about the cross-model relations with the specified keys
// in the local model.
func (c *Client) Relations(keys []string) ([]params.RelationResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(keys))}
	for i, key := range keys {
		args.Entities[i].Tag = names.NewRelationTag(key).String()
	}
	var results params.RelationResults
	err := c.facade.FacadeCall("Relations", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(keys) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(keys), len(results.Results))
	}
	return results.Results, nil
}

// RemoteApplications returns the current state of the remote applications with
// the specified names in the local model.
func (c *Client) RemoteApplications(applications []string) ([]params.RemoteApplicationResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(applications))}
	for i, applicationName := range applications {
		args.Entities[i].Tag = names.NewApplicationTag(applicationName).String()
	}
	var results params.RemoteApplicationResults
	err := c.facade.FacadeCall("RemoteApplications", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(applications) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(applications), len(results.Results))
	}
	return results.Results, nil
}

// WatchRemoteApplications returns a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote applications in the model.
func (c *Client) WatchRemoteApplications() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchRemoteApplications", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchRemoteApplicationRelations returns remote relations watchers that delivers
// changes according to the addition, removal, and lifecycle changes of
// relations that the specified remote application is involved in; and also
// according to the entering, departing, and change of unit settings in
// those relations.
func (c *Client) WatchRemoteApplicationRelations(application string) (watcher.StringsWatcher, error) {
	if !names.IsValidApplication(application) {
		return nil, errors.NotValidf("application name %q", application)
	}
	applicationTag := names.NewApplicationTag(application)
	args := params.Entities{
		Entities: []params.Entity{{Tag: applicationTag.String()}},
	}

	var results params.StringsWatchResults
	err := c.facade.FacadeCall("WatchRemoteApplicationRelations", args, &results)
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
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchLocalRelationUnits returns a watcher that notifies of changes to the
// local units in the relation with the given key.
func (c *Client) WatchLocalRelationUnits(relationKey string) (watcher.RelationUnitsWatcher, error) {
	if !names.IsValidRelation(relationKey) {
		return nil, errors.NotValidf("relation key %q", relationKey)
	}
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{
		Entities: []params.Entity{{Tag: relationTag.String()}},
	}
	var results params.RelationUnitsWatchResults
	err := c.facade.FacadeCall("WatchLocalRelationUnits", args, &results)
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
