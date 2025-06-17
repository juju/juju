// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/crossmodel"
	coresecrets "github.com/juju/juju/core/secrets"
	"github.com/juju/juju/rpc/params"
)

// ExternalControllerService provides a subset of the external controller domain
// service methods.
type ExternalControllerService interface {
	// UpdateExternalController persists the input controller
	// record and associates it with the input model UUIDs.
	UpdateExternalController(ctx context.Context, ec crossmodel.ControllerInfo) error
}

// API provides access to the remote relations API facade.
type API struct {
	ControllerConfigAPI
	ecService     ExternalControllerService
	secretService SecretService
}

// NewRemoteRelationsAPI returns a new server-side API facade.
func NewRemoteRelationsAPI(
	ecService ExternalControllerService,
	secretService SecretService,
	controllerCfgAPI ControllerConfigAPI,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthController() {
		return nil, apiservererrors.ErrPerm
	}
	return &API{
		ecService:           ecService,
		secretService:       secretService,
		ControllerConfigAPI: controllerCfgAPI,
	}, nil
}

// ImportRemoteEntities adds entities to the remote entities collection with the specified opaque tokens.
func (api *API) ImportRemoteEntities(ctx context.Context, args params.RemoteEntityTokenArgs) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// ExportEntities allocates unique, remote entity IDs for the given entities in the local model.
func (api *API) ExportEntities(ctx context.Context, entities params.Entities) (params.TokenResults, error) {
	return params.TokenResults{}, nil
}

// GetTokens returns the token associated with the entities with the given tags for the given models.
func (api *API) GetTokens(ctx context.Context, args params.GetTokenArgs) (params.StringResults, error) {
	return params.StringResults{}, nil
}

// SaveMacaroons saves the macaroons for the given entities.
func (api *API) SaveMacaroons(ctx context.Context, args params.EntityMacaroonArgs) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// Relations returns information about the cross-model relations with the specified keys
// in the local model.
func (api *API) Relations(ctx context.Context, entities params.Entities) (params.RemoteRelationResults, error) {
	return params.RemoteRelationResults{}, nil
}

// RemoteApplications returns the current state of the remote applications with
// the specified names in the local model.
func (api *API) RemoteApplications(ctx context.Context, entities params.Entities) (params.RemoteApplicationResults, error) {
	return params.RemoteApplicationResults{}, nil
}

// WatchRemoteApplications starts a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote applications in the model; and
// returns the watcher ID and initial IDs of remote applications, or an error if
// watching failed.
func (api *API) WatchRemoteApplications(ctx context.Context) (params.StringsWatchResult, error) {
	return params.StringsWatchResult{}, nil
}

// WatchLocalRelationChanges starts a RemoteRelationWatcher for each
// specified relation, returning the watcher IDs and initial values,
// or an error if the remote relations couldn't be watched.
func (api *API) WatchLocalRelationChanges(ctx context.Context, args params.Entities) (params.RemoteRelationWatchResults, error) {
	return params.RemoteRelationWatchResults{}, nil
}

// WatchRemoteApplicationRelations starts a StringsWatcher for watching the relations of
// each specified application in the local model, and returns the watcher IDs
// and initial values, or an error if the services' relations could not be
// watched.
func (api *API) WatchRemoteApplicationRelations(ctx context.Context, args params.Entities) (params.StringsWatchResults, error) {
	return params.StringsWatchResults{}, nil
}

// WatchRemoteRelations starts a strings watcher that notifies of the addition,
// removal, and lifecycle changes of remote relations in the model; and
// returns the watcher ID and initial IDs of remote relations, or an error if
// watching failed.
func (api *API) WatchRemoteRelations(ctx context.Context) (params.StringsWatchResult, error) {
	return params.StringsWatchResult{}, nil
}

// ConsumeRemoteRelationChanges consumes changes to settings originating
// from the remote/offering side of relations.
func (api *API) ConsumeRemoteRelationChanges(ctx context.Context, changes params.RemoteRelationsChanges) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// SetRemoteApplicationsStatus sets the status for the specified remote applications.
func (api *API) SetRemoteApplicationsStatus(ctx context.Context, args params.SetStatus) (params.ErrorResults, error) {
	return params.ErrorResults{}, nil
}

// UpdateControllersForModels changes the external controller records for the
// associated model entities. This is used when the remote relations worker gets
// redirected following migration of an offering model.
func (api *API) UpdateControllersForModels(ctx context.Context, args params.UpdateControllersForModelsParams) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Changes))

	for i, change := range args.Changes {
		cInfo := change.Info

		modelTag, err := names.ParseModelTag(change.ModelTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		controllerTag, err := names.ParseControllerTag(cInfo.ControllerTag)
		if err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}

		controller := crossmodel.ControllerInfo{
			ControllerUUID: controllerTag.Id(),
			Alias:          cInfo.Alias,
			Addrs:          cInfo.Addrs,
			CACert:         cInfo.CACert,
			ModelUUIDs:     []string{modelTag.Id()},
		}

		if err := api.ecService.UpdateExternalController(ctx, controller); err != nil {
			result.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
	}

	return result, nil
}

// ConsumeRemoteSecretChanges updates the local model with secret revision changes
// originating from the remote/offering model.
func (api *API) ConsumeRemoteSecretChanges(ctx context.Context, args params.LatestSecretRevisionChanges) (params.ErrorResults, error) {
	var result params.ErrorResults
	result.Results = make([]params.ErrorResult, len(args.Changes))
	for i, arg := range args.Changes {
		err := api.consumeOneRemoteSecretChange(ctx, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *API) consumeOneRemoteSecretChange(ctx context.Context, arg params.SecretRevisionChange) error {
	uri, err := coresecrets.ParseURI(arg.URI)
	if err != nil {
		return errors.Trace(err)
	}
	err = api.secretService.UpdateRemoteSecretRevision(ctx, uri, arg.LatestRevision)
	return errors.Trace(err)
}
