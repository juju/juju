// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
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

// ImportRemoteEntity adds an entity to the remote entities collection
// with the specified opaque token.
func (c *Client) ImportRemoteEntity(entity names.Tag, token string) error {
	args := params.RemoteEntityTokenArgs{Args: []params.RemoteEntityTokenArg{
		{Tag: entity.String(), Token: token}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("ImportRemoteEntities", args, &results)
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
func (c *Client) ExportEntities(tags []names.Tag) ([]params.TokenResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.TokenResults
	err := c.facade.FacadeCall("ExportEntities", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// GetToken returns the token associated with the entity with the given tag for the specified model.
func (c *Client) GetToken(tag names.Tag) (string, error) {
	args := params.GetTokenArgs{Args: []params.GetTokenArg{
		{Tag: tag.String()}},
	}
	var results params.StringResults
	err := c.facade.FacadeCall("GetTokens", args, &results)
	if err != nil {
		return "", errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return "", errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		if params.IsCodeNotFound(result.Error) {
			return "", errors.NotFoundf("token for %v", tag)
		}
		return "", errors.Trace(result.Error)
	}
	return result.Result, nil
}

// SaveMacaroon saves the macaroon for the entity.
func (c *Client) SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error {
	args := params.EntityMacaroonArgs{Args: []params.EntityMacaroonArg{
		{Tag: entity.String(), Macaroon: mac}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("SaveMacaroons", args, &results)
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

// Relations returns information about the cross-model relations with the specified keys
// in the local model.
func (c *Client) Relations(keys []string) ([]params.RemoteRelationResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(keys))}
	for i, key := range keys {
		args.Entities[i].Tag = names.NewRelationTag(key).String()
	}
	var results params.RemoteRelationResults
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

// WatchLocalRelationChanges returns a watcher that emits
// fully-expanded changes (suitable for shipping over to a different
// controller) to the local units in the relation with the given key.
func (c *Client) WatchLocalRelationChanges(relationKey string) (apiwatcher.RemoteRelationWatcher, error) {
	if !names.IsValidRelation(relationKey) {
		return nil, errors.NotValidf("relation key %q", relationKey)
	}
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{
		Entities: []params.Entity{{Tag: relationTag.String()}},
	}
	var results params.RemoteRelationWatchResults
	err := c.facade.FacadeCall("WatchLocalRelationChanges", args, &results)
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
	w := apiwatcher.NewRemoteRelationWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// WatchRemoteRelations returns a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote relations in the model.
func (c *Client) WatchRemoteRelations() (watcher.StringsWatcher, error) {
	var result params.StringsWatchResult
	err := c.facade.FacadeCall("WatchRemoteRelations", nil, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result.Error != nil {
		return nil, result.Error
	}
	w := apiwatcher.NewStringsWatcher(c.facade.RawAPICaller(), result)
	return w, nil
}

// ConsumeRemoteRelationChange consumes a change to settings originating
// from the remote/offering side of a relation.
func (c *Client) ConsumeRemoteRelationChange(change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall("ConsumeRemoteRelationChanges", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// ControllerAPIInfoForModel retrieves the controller API info for the specified model.
func (c *Client) ControllerAPIInfoForModel(modelUUID string) (*api.Info, error) {
	modelTag := names.NewModelTag(modelUUID)
	args := params.Entities{Entities: []params.Entity{{Tag: modelTag.String()}}}
	var results params.ControllerAPIInfoResults
	err := c.facade.FacadeCall("ControllerAPIInfoForModels", args, &results)
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
	return &api.Info{
		Addrs:    result.Addresses,
		CACert:   result.CACert,
		ModelTag: modelTag,
	}, nil
}

// SetRemoteApplicationStatus sets the status for the specified remote application.
func (c *Client) SetRemoteApplicationStatus(applicationName string, status status.Status, message string) error {
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: names.NewApplicationTag(applicationName).String(), Status: status.String(), Info: message},
	}}
	var results params.ErrorResults
	err := c.facade.FacadeCall("SetRemoteApplicationsStatus", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UpdateControllerForModel ensures that there is an external controller record
// for the input info, associated with the input model ID.
func (c *Client) UpdateControllerForModel(controller crossmodel.ControllerInfo, modelUUID string) error {
	args := params.UpdateControllersForModelsParams{Changes: []params.UpdateControllerForModel{{
		ModelTag: names.NewModelTag(modelUUID).String(),
		Info: params.ExternalControllerInfo{
			ControllerTag: controller.ControllerTag.String(),
			Alias:         controller.Alias,
			Addrs:         controller.Addrs,
			CACert:        controller.CACert,
		},
	}}}

	var results params.ErrorResults
	err := c.facade.FacadeCall("UpdateControllersForModels", args, &results)
	if err != nil {
		return errors.Trace(err)
	}

	if len(results.Results) != 1 {
		return errors.Errorf("expected 1 result, got %d", len(results.Results))
	}

	result := results.Results[0]
	if result.Error != nil {
		return result.Error
	}
	return nil
}
