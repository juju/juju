// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apiwatcher "github.com/juju/juju/api/watcher"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/core/watcher"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

const remoteRelationsFacade = "RemoteRelations"

// Client provides access to the remoterelations api facade.
type Client struct {
	facade base.FacadeCaller
}

// NewClient creates a new client-side RemoteRelations facade.
func NewClient(caller base.APICaller, options ...Option) *Client {
	facadeCaller := base.NewFacadeCaller(caller, remoteRelationsFacade, options...)
	return &Client{facadeCaller}
}

// ImportRemoteEntity adds an entity to the remote entities collection
// with the specified opaque token.
func (c *Client) ImportRemoteEntity(ctx context.Context, entity names.Tag, token string) error {
	args := params.RemoteEntityTokenArgs{Args: []params.RemoteEntityTokenArg{
		{Tag: entity.String(), Token: token}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "ImportRemoteEntities", args, &results)
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
func (c *Client) ExportEntities(ctx context.Context, tags []names.Tag) ([]params.TokenResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(tags))}
	for i, tag := range tags {
		args.Entities[i].Tag = tag.String()
	}
	var results params.TokenResults
	err := c.facade.FacadeCall(ctx, "ExportEntities", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != len(tags) {
		return nil, errors.Errorf("expected %d result(s), got %d", len(tags), len(results.Results))
	}
	return results.Results, nil
}

// GetToken returns the token associated with the entity with the given tag for the specified model.
func (c *Client) GetToken(ctx context.Context, tag names.Tag) (string, error) {
	args := params.GetTokenArgs{Args: []params.GetTokenArg{
		{Tag: tag.String()}},
	}
	var results params.StringResults
	err := c.facade.FacadeCall(ctx, "GetTokens", args, &results)
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
func (c *Client) SaveMacaroon(ctx context.Context, entity names.Tag, mac *macaroon.Macaroon) error {
	args := params.EntityMacaroonArgs{Args: []params.EntityMacaroonArg{
		{Tag: entity.String(), Macaroon: mac}},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "SaveMacaroons", args, &results)
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
func (c *Client) Relations(ctx context.Context, keys []string) ([]params.RemoteRelationResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(keys))}
	for i, key := range keys {
		args.Entities[i].Tag = names.NewRelationTag(key).String()
	}
	var results params.RemoteRelationResults
	err := c.facade.FacadeCall(ctx, "Relations", args, &results)
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
func (c *Client) RemoteApplications(ctx context.Context, applications []string) ([]params.RemoteApplicationResult, error) {
	args := params.Entities{Entities: make([]params.Entity, len(applications))}
	for i, applicationName := range applications {
		args.Entities[i].Tag = names.NewApplicationTag(applicationName).String()
	}
	var results params.RemoteApplicationResults
	err := c.facade.FacadeCall(ctx, "RemoteApplications", args, &results)
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
func (c *Client) WatchRemoteApplications(ctx context.Context) (watcher.StringsWatcher, error) {
	// todo(gfouillet): re-enable this watcher call whenever CMR will be fully
	//   implemented in the new domain. It is required to disable that way
	//   because this watcher is required to allows the uniter to run.
	return newDisabledWatcher(), nil
}

// WatchRemoteApplicationRelations returns remote relations watchers that delivers
// changes according to the addition, removal, and lifecycle changes of
// relations that the specified remote application is involved in; and also
// according to the entering, departing, and change of unit settings in
// those relations.
func (c *Client) WatchRemoteApplicationRelations(ctx context.Context, application string) (watcher.StringsWatcher, error) {
	// todo(gfouillet): re-enable this watcher call whenever CMR will be fully
	//   implemented in the new domain. It is required to disable that way
	//   because this watcher is required to allows the uniter to run.
	return newDisabledWatcher(), nil
}

// WatchLocalRelationChanges returns a watcher that emits
// fully-expanded changes (suitable for shipping over to a different
// controller) to the local units in the relation with the given key.
func (c *Client) WatchLocalRelationChanges(ctx context.Context, relationKey string) (apiwatcher.RemoteRelationWatcher, error) {
	if !names.IsValidRelation(relationKey) {
		return nil, errors.NotValidf("relation key %q", relationKey)
	}
	relationTag := names.NewRelationTag(relationKey)
	args := params.Entities{
		Entities: []params.Entity{{Tag: relationTag.String()}},
	}
	var results params.RemoteRelationWatchResults
	err := c.facade.FacadeCall(ctx, "WatchLocalRelationChanges", args, &results)
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
func (c *Client) WatchRemoteRelations(ctx context.Context) (watcher.StringsWatcher, error) {
	// todo(gfouillet): re-enable this watcher call whenever CMR will be fully
	//   implemented in the new domain. It is required to disable that way
	//   because this watcher is required to allows the uniter to run.
	return newDisabledWatcher(), nil
}

// ConsumeRemoteRelationChange consumes a change to settings originating
// from the remote/offering side of a relation.
func (c *Client) ConsumeRemoteRelationChange(ctx context.Context, change params.RemoteRelationChangeEvent) error {
	args := params.RemoteRelationsChanges{
		Changes: []params.RemoteRelationChangeEvent{change},
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "ConsumeRemoteRelationChanges", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// ControllerAPIInfoForModel retrieves the controller API info for the specified model.
func (c *Client) ControllerAPIInfoForModel(ctx context.Context, modelUUID string) (*api.Info, error) {
	modelTag := names.NewModelTag(modelUUID)
	args := params.Entities{Entities: []params.Entity{{Tag: modelTag.String()}}}
	var results params.ControllerAPIInfoResults
	err := c.facade.FacadeCall(ctx, "ControllerAPIInfoForModels", args, &results)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if len(results.Results) != 1 {
		return nil, errors.Errorf("expected 1 result, got %d", len(results.Results))
	}
	result := results.Results[0]
	if result.Error != nil {
		return nil, apiservererrors.RestoreError(result.Error)
	}
	return &api.Info{
		Addrs:    result.Addresses,
		CACert:   result.CACert,
		ModelTag: modelTag,
	}, nil
}

// SetRemoteApplicationStatus sets the status for the specified remote application.
func (c *Client) SetRemoteApplicationStatus(ctx context.Context, applicationName string, status status.Status, message string) error {
	args := params.SetStatus{Entities: []params.EntityStatusArgs{
		{Tag: names.NewApplicationTag(applicationName).String(), Status: status.String(), Info: message},
	}}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "SetRemoteApplicationsStatus", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.OneError()
}

// UpdateControllerForModel ensures that there is an external controller record
// for the input info, associated with the input model ID.
func (c *Client) UpdateControllerForModel(ctx context.Context, controller crossmodel.ControllerInfo, modelUUID string) error {
	args := params.UpdateControllersForModelsParams{Changes: []params.UpdateControllerForModel{{
		ModelTag: names.NewModelTag(modelUUID).String(),
		Info: params.ExternalControllerInfo{
			ControllerTag: names.NewControllerTag(controller.ControllerUUID).String(),
			Alias:         controller.Alias,
			Addrs:         controller.Addrs,
			CACert:        controller.CACert,
		},
	}}}

	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "UpdateControllersForModels", args, &results)
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

// ConsumeRemoteSecretChanges updates the local model with secret revision  changes
// originating from the remote/offering model.
func (c *Client) ConsumeRemoteSecretChanges(ctx context.Context, changes []watcher.SecretRevisionChange) error {
	if len(changes) == 0 {
		return nil
	}
	args := params.LatestSecretRevisionChanges{
		Changes: make([]params.SecretRevisionChange, len(changes)),
	}
	for i, c := range changes {
		args.Changes[i] = params.SecretRevisionChange{
			URI:            c.URI.String(),
			LatestRevision: c.Revision,
		}
	}
	var results params.ErrorResults
	err := c.facade.FacadeCall(ctx, "ConsumeRemoteSecretChanges", args, &results)
	if err != nil {
		return errors.Trace(err)
	}
	return results.Combine()
}
