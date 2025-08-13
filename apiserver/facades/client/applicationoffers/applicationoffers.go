// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"

	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corecrossmodel "github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/offer"
	offererrors "github.com/juju/juju/domain/offer/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// OffersAPIv5 implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPIv5 struct {
	*OffersAPI
}

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	authorizer     facade.Authorizer
	controllerUUID string
	modelUUID      model.UUID

	accessService AccessService
	modelService  ModelService

	offerServiceGetter   func(c context.Context, modelUUID model.UUID) (OfferService, error)
	removalServiceGetter func(c context.Context, modelUUID model.UUID) (RemovalService, error)
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	authorizer facade.Authorizer,
	controllerUUID string,
	modelUUID model.UUID,
	accessService AccessService,
	modelService ModelService,
	offerServiceGetter func(c context.Context, modelUUID model.UUID) (OfferService, error),
	removalServiceGetter func(c context.Context, modelUUID model.UUID) (RemovalService, error),
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	api := &OffersAPI{
		authorizer:           authorizer,
		controllerUUID:       controllerUUID,
		modelUUID:            modelUUID,
		accessService:        accessService,
		modelService:         modelService,
		offerServiceGetter:   offerServiceGetter,
		removalServiceGetter: removalServiceGetter,
	}
	return api, nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPI) Offer(ctx context.Context, all params.AddApplicationOffers) (params.ErrorResults, error) {
	// Although this API is offering adding offers in bulk, we only want to
	// support adding one offer at a time. This is because we're jumping into
	// other models using the state pool, in the context of a model facade.
	// There is no limit, nor pagination, on the number of offers that can be
	// added in one call, so any nefarious user could add a large number of
	// offers in one call, and potentially exhaust the state pool. This becomes
	// more of a problem when we move to dqlite (4.0 and beyond), as each
	// model is within a different database. By limiting the number of offers
	// we force the clients to make multiple calls and if required we can
	// enforce rate limiting.
	// This API will be deprecated in the future and replaced once we refactor
	// the API (5.0 and beyond).
	numOffers := len(all.Offers)
	if numOffers != 1 {
		return params.ErrorResults{}, errors.Errorf("expected exactly one offer, got %d", numOffers)
	}

	handleErr := func(err error) params.ErrorResults {
		return params.ErrorResults{Results: []params.ErrorResult{{
			Error: apiservererrors.ServerError(err),
		}}}
	}

	apiUserTag, ok := api.authorizer.GetAuthTag().(names.UserTag)
	if !ok {
		return handleErr(apiservererrors.ErrPerm), nil
	}

	one := all.Offers[0]
	offerModelUUID := api.modelUUID
	if one.ModelTag != "" {
		modelTag, err := names.ParseModelTag(one.ModelTag)
		if err != nil {
			return handleErr(err), nil
		}
		offerModelUUID = model.UUID(modelTag.Id())
	}

	// checkAPIUserAdmin on the offer model.
	if err := api.checkAPIUserAdmin(ctx, offerModelUUID); err != nil {
		msgerr := errors.Errorf("checking user %q has admin permission on model %q: %w", apiUserTag.String(), offerModelUUID.String(), apiservererrors.ErrPerm)
		return handleErr(msgerr), nil
	}

	applicationOfferArgs, ownerTag, err := api.parseApplicationOfferArgs(apiUserTag, one)
	if err != nil {
		return handleErr(err), nil
	}

	// If the owner and caller are not the same, checkAdmin for the offer model.
	if apiUserTag != ownerTag {
		if err := api.checkAdmin(ctx, ownerTag, offerModelUUID); err != nil {
			msgerr := errors.Errorf("checking user %q has admin permission on model %q: %w", ownerTag.String(), offerModelUUID.String(), err).Add(coreerrors.NotValid)
			return handleErr(msgerr), nil
		}
	}

	offerService, err := api.offerServiceGetter(ctx, offerModelUUID)
	if err != nil {
		return handleErr(err), nil
	}

	err = offerService.Offer(ctx, applicationOfferArgs)
	return handleErr(err), nil
}

func (api *OffersAPI) parseApplicationOfferArgs(
	apiUser names.UserTag,
	addOfferParams params.AddApplicationOffer,
) (offer.ApplicationOfferArgs, names.UserTag, error) {
	owner := apiUser
	if addOfferParams.OwnerTag != "" {
		var err error
		if owner, err = names.ParseUserTag(addOfferParams.OwnerTag); err != nil {
			return offer.ApplicationOfferArgs{}, names.UserTag{}, err
		}
	}
	result := offer.ApplicationOfferArgs{
		OfferName:       addOfferParams.OfferName,
		ApplicationName: addOfferParams.ApplicationName,
		Endpoints:       addOfferParams.Endpoints,
		OwnerName:       coreuser.NameFromTag(owner),
	}
	return result, owner, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPI) ListApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	return params.QueryApplicationOffersResultsV5{}, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPI) ModifyOfferAccess(ctx context.Context, args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	// Delay checking permission until the models are known. The api user
	// must have one of the following:
	// * superuser access to the controller
	// * admin access for the model
	// * admin access for the offer
	// Offer access is kept in the controller database, not in a model database.

	offerURLs := make([]string, len(args.Changes))
	for i, arg := range args.Changes {
		offerURLs[i] = arg.OfferURL
	}
	apiUserTag, ok := api.authorizer.GetAuthTag().(names.UserTag)
	if !ok {
		return result, apiservererrors.ErrPerm
	}
	models, err := api.getModelsFromOffers(ctx, apiUserTag, offerURLs...)
	if err != nil {
		return result, errors.Capture(err)
	}

	for i, arg := range args.Changes {
		if models[i].err != nil {
			result.Results[i].Error = apiservererrors.ServerError(models[i].err)
			continue
		}
		err = api.modifyOneOfferAccess(
			ctx,
			apiUserTag,
			models[i].url,
			models[i].model.UUID.String(),
			arg,
		)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *OffersAPI) modifyOneOfferAccess(
	ctx context.Context,
	apiUserTag names.UserTag,
	offerURL *corecrossmodel.OfferURL,
	modelUUID string,
	arg params.ModifyOfferAccess,
) error {

	offerService, err := api.offerServiceGetter(ctx, model.UUID(modelUUID))
	if err != nil {
		return errors.Capture(err)
	}

	offerUUID, err := offerService.GetOfferUUID(ctx, offerURL)
	if errors.Is(err, offererrors.OfferNotFound) {
		return apiservererrors.ParamsErrorf(params.CodeNotFound, "offer %q not found", offerURL)
	} else if err != nil {
		return errors.Errorf("getting offer uuid of %q: %w", offerURL.String(), err)
	}

	// The api user must have superuser permission on the controller,
	// admin permission on the model or offer to continue.
	if err := api.checkAPIUserAdmin(ctx, model.UUID(modelUUID)); err != nil {
		err = api.authorizer.EntityHasPermission(ctx, apiUserTag, permission.AdminAccess, names.NewApplicationOfferTag(offerUUID.String()))
		if err != nil {
			return apiservererrors.ErrPerm
		}
	}

	return api.changeOfferAccess(ctx, offerUUID.String(), arg.UserTag, arg.Action, permission.Access(arg.Access))
}

// changeOfferAccess performs the requested access grant or revoke action for the
// specified user on the specified application offer.
func (api *OffersAPI) changeOfferAccess(
	ctx context.Context,
	offerUUID string,
	targetUser string,
	action params.OfferAction,
	accessLevel permission.Access,
) error {
	targetUserTag, err := names.ParseUserTag(targetUser)
	if err != nil {
		return errors.Capture(err)
	}
	targetUserName := coreuser.NameFromTag(targetUserTag)

	var change permission.AccessChange
	switch action {
	case params.GrantOfferAccess:
		change = permission.Grant
	case params.RevokeOfferAccess:
		change = permission.Revoke
	default:
		return errors.Errorf("unknown action %q", action)
	}

	err = api.accessService.UpdatePermission(ctx, access.UpdatePermissionArgs{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerUUID,
			},
			Access: accessLevel,
		},
		Change:  change,
		Subject: targetUserName,
	})
	if err != nil {
		return errors.Errorf("could not %s offer access for %q: %w", change, targetUserName, err)
	}
	return nil
}

type offerModel struct {
	url   *corecrossmodel.OfferURL
	model model.Model
	err   error
}

// getModelsFromOffers returns a slice of models corresponding to the
// specified offer URLs. Each result item has either a model or an error.
func (api *OffersAPI) getModelsFromOffers(ctx context.Context, user names.UserTag, offerURLs ...string) ([]offerModel, error) {
	// Cache the models found so far so we don't look them up more than once.
	modelsCache := make(map[string]model.Model)
	oneModel := func(offerURL string) (*corecrossmodel.OfferURL, model.Model, error) {
		url, err := corecrossmodel.ParseOfferURL(offerURL)
		if err != nil {
			return nil, model.Model{}, errors.Capture(err)
		}
		modelPath := fmt.Sprintf("%s/%s", url.ModelQualifier, url.ModelName)
		if foundModel, ok := modelsCache[modelPath]; ok {
			return url, foundModel, nil
		}

		modelQualifier := url.ModelQualifier
		if modelQualifier == "" {
			modelQualifier = user.Id()
		}
		m, err := api.modelForName(ctx, url.ModelName, modelQualifier)
		if err != nil {
			return nil, model.Model{}, errors.Capture(err)
		}
		return url, m, nil
	}

	result := make([]offerModel, len(offerURLs))
	for i, offerURL := range offerURLs {
		var om offerModel
		om.url, om.model, om.err = oneModel(offerURL)
		result[i] = om
	}
	return result, nil
}

// modelForName returns the model details for the specified model name,
// along with the absolute model path used in the lookup.
//
// The following errors may be returned:
// - [coreerrors.NotFound] when no model with the given name exists.
// - [coreerrors.NotValid] when ownerName is not valid.
func (api *OffersAPI) modelForName(ctx context.Context, modelName, ownerName string) (model.Model, error) {
	qualifier := model.QualifierFromUserTag(names.NewUserTag(ownerName))
	m, err := api.modelService.GetModelByNameAndQualifier(ctx, modelName, qualifier)
	if errors.Is(err, modelerrors.NotFound) {
		return model.Model{}, errors.Errorf(`model "%s/%s": %w`, ownerName, modelName, coreerrors.NotFound)
	} else if errors.Is(err, accesserrors.UserNameNotValid) {
		return model.Model{}, errors.Errorf("user name %q: %w", ownerName, coreerrors.NotValid)
	}

	return m, errors.Capture(err)
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *OffersAPI) ApplicationOffers(ctx context.Context, urls params.OfferURLs) (params.ApplicationOffersResults, error) {

	return params.ApplicationOffersResults{}, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *OffersAPI) FindApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	return params.QueryApplicationOffersResultsV5{}, nil
}

// GetConsumeDetails returns the details necessary to pass to another model
// to allow the specified args user to consume the offers represented by the args URLs.
func (api *OffersAPI) GetConsumeDetails(ctx context.Context, args params.ConsumeOfferDetailsArg) (params.ConsumeOfferDetailsResults, error) {
	return params.ConsumeOfferDetailsResults{}, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
// This call currently has no client side API, only there for the Dashboard at this stage.
func (api *OffersAPI) RemoteApplicationInfo(ctx context.Context, args params.OfferURLs) (params.RemoteApplicationInfoResults, error) {
	return params.RemoteApplicationInfoResults{}, nil
}

// DestroyOffers removes the offers specified by the given URLs, forcing if necessary.
func (api *OffersAPI) DestroyOffers(ctx context.Context, args params.DestroyApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.OfferURLs))

	user, ok := api.authorizer.GetAuthTag().(names.UserTag)
	if !ok {
		return params.ErrorResults{}, apiservererrors.ErrPerm
	}
	models, err := api.getModelsFromOffers(ctx, user, args.OfferURLs...)
	if err != nil {
		return params.ErrorResults{}, errors.Capture(err)
	}

	for i, one := range args.OfferURLs {
		err := api.destroyOneOffer(ctx, models[i], one, args.Force)
		result[i].Error = apiservererrors.ServerError(err)
	}

	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPI) destroyOneOffer(ctx context.Context, offerModel offerModel, offerString string, force bool) error {
	if offerModel.err != nil {
		return offerModel.err
	}

	url, err := corecrossmodel.ParseOfferURL(offerString)
	if err != nil {
		return err
	}

	modelUUID := offerModel.model.UUID
	if err := api.checkAPIUserAdmin(ctx, modelUUID); err != nil {
		return apiservererrors.ErrPerm
	}

	offerService, err := api.offerServiceGetter(ctx, modelUUID)
	if err != nil {
		return err
	}

	removalService, err := api.removalServiceGetter(ctx, modelUUID)
	if err != nil {
		return err
	}

	offerUUID, err := offerService.GetOfferUUID(ctx, url)
	if err != nil {
		return err
	}

	return removalService.RemoveOffer(ctx, offerUUID, force)
}

// checkAdmin ensures that the specified in user is a model or controller admin.
func (api *OffersAPI) checkAdmin(
	ctx context.Context, user names.UserTag, modelUUID model.UUID,
) error {
	controllerTag := names.NewControllerTag(api.controllerUUID)
	err := api.authorizer.EntityHasPermission(ctx, user, permission.SuperuserAccess, controllerTag)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		err = api.authorizer.EntityHasPermission(ctx, user, permission.AdminAccess, names.NewModelTag(modelUUID.String()))
	}
	return errors.Capture(err)
}

// checkAdmin ensures that the specified in user is a model or controller admin.
func (api *OffersAPI) checkAPIUserAdmin(
	ctx context.Context, modelUUID model.UUID,
) error {
	controllerTag := names.NewControllerTag(api.controllerUUID)
	err := api.authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
	if errors.Is(err, authentication.ErrorEntityMissingPermission) {
		err = api.authorizer.HasPermission(ctx, permission.AdminAccess, names.NewModelTag(modelUUID.String()))
	}

	return errors.Capture(err)
}
