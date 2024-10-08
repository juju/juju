// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	access "github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/rpc/params"
)

// OffersAPIv5 implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPIv5 struct {
	BaseAPI
	dataDir     string
	authContext *commoncrossmodel.AuthContext
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	getControllerInfo func(context.Context) ([]string, string, error),
	backend Backend,
	statePool StatePool,
	accessService AccessService,
	modelDomainServicesGetter ModelDomainServicesGetter,
	authorizer facade.Authorizer,
	authContext *commoncrossmodel.AuthContext,
	dataDir string,
	logger corelogger.Logger,
	controllerUUID string,
	modelUUID model.UUID,
) (*OffersAPIv5, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	api := &OffersAPIv5{
		dataDir:     dataDir,
		authContext: authContext,
		BaseAPI: BaseAPI{
			Authorizer:                authorizer,
			GetApplicationOffers:      getApplicationOffers,
			ControllerModel:           backend,
			accessService:             accessService,
			modelDomainServicesGetter: modelDomainServicesGetter,
			StatePool:                 statePool,
			getControllerInfo:         getControllerInfo,
			logger:                    logger,
			controllerUUID:            controllerUUID,
			modelUUID:                 modelUUID,
		},
	}
	return api, nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPIv5) Offer(ctx context.Context, all params.AddApplicationOffers) (params.ErrorResults, error) {
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

	apiUser := api.Authorizer.GetAuthTag().(names.UserTag)

	one := all.Offers[0]
	modelTag, err := names.ParseModelTag(one.ModelTag)
	if err != nil {
		return handleErr(err), nil
	}

	backend, releaser, err := api.StatePool.Get(modelTag.Id())
	if err != nil {
		return handleErr(err), nil
	}
	defer releaser()

	modelUUID := model.UUID(modelTag.Id())

	if err := api.checkAdmin(ctx, apiUser, modelUUID, backend); err != nil {
		return handleErr(err), nil
	}

	owner := apiUser
	// The V4 version of the api includes the offer owner in the params.
	if one.OwnerTag != "" {
		var err error
		if owner, err = names.ParseUserTag(one.OwnerTag); err != nil {
			return handleErr(err), nil
		}
	}

	modelDomainServices := api.modelDomainServicesGetter.DomainServicesForModel(modelUUID)
	applicationOfferParams, err := api.makeAddOfferArgsFromParams(ctx, owner, modelDomainServices.Application(), one)
	if err != nil {
		return handleErr(err), nil
	}

	offerBackend := api.GetApplicationOffers(backend)
	if _, err = offerBackend.ApplicationOffer(applicationOfferParams.OfferName); err == nil {
		_, err = offerBackend.UpdateOffer(applicationOfferParams)
		if err != nil {
			return handleErr(err), nil
		}
	} else {
		var addedOffer *jujucrossmodel.ApplicationOffer
		addedOffer, err = offerBackend.AddOffer(applicationOfferParams)
		if err != nil {
			return handleErr(err), nil
		}

		// TODO(aflynn50): New offer permissions are handled here as there
		// is no cross model relations service yet. These should be handled
		// there once its created.
		offerTag := names.NewApplicationOfferTag(addedOffer.OfferUUID)
		// Ensure the owner has admin access to the offer.
		err = api.createOfferPermission(ctx, offerTag, permission.AdminAccess, owner)
		if err != nil {
			return handleErr(err), nil
		}
		// Add in any read access permissions.
		for _, readerName := range applicationOfferParams.HasRead {
			readerTag := names.NewUserTag(readerName)
			err = api.createOfferPermission(ctx, offerTag, permission.ReadAccess, readerTag)
			if err != nil {
				return handleErr(err), nil
			}
		}
	}
	return handleErr(err), nil
}

func (api *OffersAPIv5) makeAddOfferArgsFromParams(ctx context.Context, user names.UserTag, applicationService ApplicationService, addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
		Owner:                  user.Id(),
		HasRead:                []string{permission.EveryoneUserName.Name()},
	}
	if result.OfferName == "" {
		result.OfferName = result.ApplicationName
	}

	// If we have the full result, just return early.
	if result.ApplicationDescription != "" {
		return result, nil
	}

	charmID, err := applicationService.GetCharmIDByApplicationName(context.Background(), result.ApplicationName)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return result, errors.NotFoundf("getting offered application %q", result.ApplicationName)
	} else if err != nil {
		return result, errors.Annotatef(err, "getting charm ID for application %v", result.ApplicationName)
	}

	description, err := applicationService.GetCharmMetadataDescription(ctx, charmID)
	if errors.Is(err, applicationerrors.ApplicationNotFound) {
		return result, errors.NotFoundf("getting offered application %q", result.ApplicationName)
	} else if errors.Is(err, applicationerrors.CharmNotFound) {
		return result, errors.NotFoundf("getting offered application %q charm", result.ApplicationName)
	} else if err != nil {
		return result, errors.Annotatef(err, "getting charm description for application %v", result.ApplicationName)
	}

	result.ApplicationDescription = description

	return result, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPIv5) ListApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	var result params.QueryApplicationOffersResultsV5
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	offers, err := api.getApplicationOffersDetails(ctx, user, filters, permission.AdminAccess)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPIv5) ModifyOfferAccess(ctx context.Context, args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	err := api.Authorizer.HasPermission(ctx, permission.SuperuserAccess, api.ControllerModel.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return result, errors.Trace(err)
	}
	isControllerAdmin := err == nil

	offerURLs := make([]string, len(args.Changes))
	for i, arg := range args.Changes {
		offerURLs[i] = arg.OfferURL
	}
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	models, err := api.getModelsFromOffers(user, offerURLs...)
	if err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Changes {
		if models[i].err != nil {
			result.Results[i].Error = apiservererrors.ServerError(models[i].err)
			continue
		}
		err = api.modifyOneOfferAccess(ctx, user, models[i].model.UUID(), isControllerAdmin, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *OffersAPIv5) modifyOneOfferAccess(ctx context.Context, user names.UserTag, modelUUID string, isControllerAdmin bool, arg params.ModifyOfferAccess) error {
	backend, releaser, err := api.StatePool.Get(modelUUID)
	if err != nil {
		return errors.Trace(err)
	}
	defer releaser()

	offerAccess := permission.Access(arg.Access)
	if err := permission.ValidateOfferAccess(offerAccess); err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}

	url, err := jujucrossmodel.ParseOfferURL(arg.OfferURL)
	if err != nil {
		return errors.Trace(err)
	}

	canModifyOffer := isControllerAdmin
	if !canModifyOffer {
		err = api.Authorizer.HasPermission(ctx, permission.AdminAccess, names.NewModelTag(modelUUID))
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return errors.Trace(err)
		}
	}

	if !canModifyOffer {
		offer, err := backend.ApplicationOffer(url.ApplicationName)
		if err != nil {
			return apiservererrors.ErrPerm
		}
		accessLevel, err := api.accessService.ReadUserAccessLevelForTarget(ctx, coreuser.NameFromTag(user), permission.ID{
			ObjectType: permission.Offer,
			Key:        offer.OfferUUID,
		})
		if err != nil && !errors.Is(err, accesserrors.AccessNotFound) {
			return errors.Trace(err)
		} else if err == nil {
			canModifyOffer = accessLevel == permission.AdminAccess
		}
	}
	if !canModifyOffer {
		return apiservererrors.ErrPerm
	}

	targetUserTag, err := names.ParseUserTag(arg.UserTag)
	if err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}
	return api.changeOfferAccess(ctx, backend, url.ApplicationName, targetUserTag, arg.Action, offerAccess)
}

// changeOfferAccess performs the requested access grant or revoke action for the
// specified user on the specified application offer.
func (api *OffersAPIv5) changeOfferAccess(
	ctx context.Context,
	backend Backend,
	offerName string,
	targetUserTag names.UserTag,
	action params.OfferAction,
	accessLevel permission.Access,
) error {
	targetUserName := coreuser.NameFromTag(targetUserTag)

	offer, err := backend.ApplicationOffer(offerName)
	if err != nil {
		return errors.Trace(err)
	}
	offerTag := names.NewApplicationOfferTag(offer.OfferUUID)

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
				Key:        offerTag.Id(),
			},
			Access: accessLevel,
		},
		Change:  change,
		Subject: targetUserName,
	})
	return errors.Annotatef(err, "could not %s offer access for %q", change, targetUserName)
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *OffersAPIv5) ApplicationOffers(ctx context.Context, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	return api.getApplicationOffers(ctx, user, urls)
}

func (api *OffersAPIv5) getApplicationOffers(ctx context.Context, user names.UserTag, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
	var results params.ApplicationOffersResults
	results.Results = make([]params.ApplicationOfferResult, len(urls.OfferURLs))

	var (
		filters []params.OfferFilter
		// fullURLs contains the URL strings from the url args,
		// with any optional parts like model owner filled in.
		// It is used to process the result offers.
		fullURLs []string
	)
	for i, urlStr := range urls.OfferURLs {
		url, err := jujucrossmodel.ParseOfferURL(urlStr)
		if err != nil {
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if url.User == "" {
			url.User = user.Id()
		}
		if url.HasEndpoint() {
			results.Results[i].Error = apiservererrors.ServerError(
				errors.Errorf("saas application %q shouldn't include endpoint", url))
			continue
		}
		if url.Source != "" {
			results.Results[i].Error = apiservererrors.ServerError(
				errors.NotSupportedf("query for non-local application offers"))
			continue
		}
		fullURLs = append(fullURLs, url.String())
		filters = append(filters, api.filterFromURL(url))
	}
	if len(filters) == 0 {
		return results, nil
	}
	offers, err := api.getApplicationOffersDetails(ctx, user, params.OfferFilters{Filters: filters}, permission.ReadAccess)
	if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	offersByURL := make(map[string]params.ApplicationOfferAdminDetailsV5)
	for _, offer := range offers {
		offersByURL[offer.OfferURL] = offer
	}
	for i, urlStr := range fullURLs {
		offer, ok := offersByURL[urlStr]
		if !ok {
			err = errors.NotFoundf("application offer %q", urlStr)
			results.Results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results.Results[i].Result = &offer
	}
	return results, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *OffersAPIv5) FindApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResultsV5, error) {
	var result params.QueryApplicationOffersResultsV5
	var filtersToUse params.OfferFilters

	// If there is only one filter term, and no model is specified, add in
	// any models the user can see and query across those.
	// If there's more than one filter term, each must specify a model.
	if len(filters.Filters) == 1 && filters.Filters[0].ModelName == "" {
		uuids, err := api.ControllerModel.AllModelUUIDs()
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, uuid := range uuids {
			m, release, err := api.StatePool.GetModel(uuid)
			if err != nil {
				return result, errors.Trace(err)
			}
			defer release()
			modelFilter := filters.Filters[0]
			modelFilter.ModelName = m.Name()
			modelFilter.OwnerName = m.Owner().Id()
			filtersToUse.Filters = append(filtersToUse.Filters, modelFilter)
		}
	} else {
		filtersToUse = filters
	}
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	offers, err := api.getApplicationOffersDetails(ctx, user, filtersToUse, permission.ReadAccess)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// GetConsumeDetails returns the details necessary to pass to another model
// to allow the specified args user to consume the offers represented by the args URLs.
func (api *OffersAPIv5) GetConsumeDetails(ctx context.Context, args params.ConsumeOfferDetailsArg) (params.ConsumeOfferDetailsResults, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	// Prefer args user if provided.
	if args.UserTag != "" {
		// Only controller admins can get consume details for another user.
		err := api.checkControllerAdmin(ctx)
		if err != nil {
			return params.ConsumeOfferDetailsResults{}, errors.Trace(err)
		}
		user, err = names.ParseUserTag(args.UserTag)
		if err != nil {
			return params.ConsumeOfferDetailsResults{}, errors.Trace(err)
		}
	}
	return api.getConsumeDetails(ctx, user, args.OfferURLs)
}

// getConsumeDetails returns the details necessary to pass to another model to
// allow the specified user to consume the specified offers represented by the
// urls.
func (api *OffersAPIv5) getConsumeDetails(ctx context.Context, user names.UserTag, urls params.OfferURLs) (params.ConsumeOfferDetailsResults, error) {
	var consumeResults params.ConsumeOfferDetailsResults
	results := make([]params.ConsumeOfferDetailsResult, len(urls.OfferURLs))

	offers, err := api.getApplicationOffers(ctx, user, urls)
	if err != nil {
		return consumeResults, apiservererrors.ServerError(err)
	}

	addrs, caCert, err := api.getControllerInfo(ctx)
	if err != nil {
		return consumeResults, apiservererrors.ServerError(err)
	}

	controllerInfo := &params.ExternalControllerInfo{
		ControllerTag: api.ControllerModel.ControllerTag().String(),
		Addrs:         addrs,
		CACert:        caCert,
	}

	for i, result := range offers.Results {
		results[i].Error = result.Error
		if result.Error != nil {
			continue
		}
		offer := result.Result
		offerDetails := &offer.ApplicationOfferDetailsV5
		results[i].Offer = offerDetails
		results[i].ControllerInfo = controllerInfo

		modelTag, err := names.ParseModelTag(offerDetails.SourceModelTag)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		backend, releaser, err := api.StatePool.Get(modelTag.Id())
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		defer releaser()

		err = api.checkAdmin(ctx, user, model.UUID(modelTag.Id()), backend)
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		isAdmin := err == nil
		if !isAdmin {
			appOffer := names.NewApplicationOfferTag(offer.OfferUUID)
			err := api.Authorizer.EntityHasPermission(ctx, user, permission.ConsumeAccess, appOffer)
			if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
				results[i].Error = apiservererrors.ServerError(err)
				continue
			}
			if err != nil {
				// This logic is purely for JaaS.
				// Jaas has already checked permissions of args.UserTag in their side, so we don't need to check it again.
				// But as a TODO, we need to set the ConsumeOfferMacaroon's expiry time to 0 to force go to
				// discharge flow once they got the macaroon.
				err := api.checkControllerAdmin(ctx)
				if err != nil {
					results[i].Error = apiservererrors.ServerError(err)
					continue
				}
			}
		}
		offerMacaroon, err := api.authContext.CreateConsumeOfferMacaroon(ctx, offerDetails, user.Id(), urls.BakeryVersion)
		if err != nil {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		results[i].Macaroon = offerMacaroon.M()
	}
	consumeResults.Results = results
	return consumeResults, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
// This call currently has no client side API, only there for the Dashboard at this stage.
func (api *OffersAPIv5) RemoteApplicationInfo(ctx context.Context, args params.OfferURLs) (params.RemoteApplicationInfoResults, error) {
	results := make([]params.RemoteApplicationInfoResult, len(args.OfferURLs))
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	for i, url := range args.OfferURLs {
		info, err := api.oneRemoteApplicationInfo(ctx, user, url)
		results[i].Result = info
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.RemoteApplicationInfoResults{Results: results}, nil
}

func (api *OffersAPIv5) filterFromURL(url *jujucrossmodel.OfferURL) params.OfferFilter {
	f := params.OfferFilter{
		OwnerName: url.User,
		ModelName: url.ModelName,
		OfferName: url.ApplicationName,
	}
	return f
}

func (api *OffersAPIv5) oneRemoteApplicationInfo(ctx context.Context, user names.UserTag, urlStr string) (*params.RemoteApplicationInfo, error) {
	url, err := jujucrossmodel.ParseOfferURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need at least read access to the model to see the application details.
	// 	offer, err := api.offeredApplicationDetails(url, permission.ReadAccess)
	offers, err := api.getApplicationOffersDetails(
		ctx,
		user,
		params.OfferFilters{Filters: []params.OfferFilter{api.filterFromURL(url)}}, permission.ConsumeAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The offers query succeeded but there were no offers matching the required offer name.
	if len(offers) == 0 {
		return nil, errors.NotFoundf("application offer %q", url.ApplicationName)
	}
	// Sanity check - this should never happen.
	if len(offers) > 1 {
		return nil, errors.Errorf("unexpected: %d matching offers for %q", len(offers), url.ApplicationName)
	}
	offer := offers[0]

	return &params.RemoteApplicationInfo{
		ModelTag:         offer.SourceModelTag,
		Name:             url.ApplicationName,
		Description:      offer.ApplicationDescription,
		OfferURL:         url.String(),
		SourceModelLabel: url.ModelName,
		Endpoints:        offer.Endpoints,
		IconURLPath:      fmt.Sprintf("rest/1.0/remote-application/%s/icon", url.ApplicationName),
	}, nil
}

// DestroyOffers removes the offers specified by the given URLs, forcing if necessary.
func (api *OffersAPIv5) DestroyOffers(ctx context.Context, args params.DestroyApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(args.OfferURLs))

	user := api.Authorizer.GetAuthTag().(names.UserTag)
	models, err := api.getModelsFromOffers(user, args.OfferURLs...)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	for i, one := range args.OfferURLs {
		url, err := jujucrossmodel.ParseOfferURL(one)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		if models[i].err != nil {
			result[i].Error = apiservererrors.ServerError(models[i].err)
			continue
		}
		backend, releaser, err := api.StatePool.Get(models[i].model.UUID())
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		defer releaser()

		modelUUID := model.UUID(models[i].model.UUID())
		if err := api.checkAdmin(ctx, user, modelUUID, backend); err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = api.GetApplicationOffers(backend).Remove(url.ApplicationName, args.Force)
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPIv5) createOfferPermission(
	ctx context.Context,
	offerTag names.ApplicationOfferTag,
	accessLevel permission.Access,
	targetUserTag names.UserTag,
) error {
	_, err := api.accessService.CreatePermission(ctx, permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Offer,
				Key:        offerTag.Id(),
			},
			Access: accessLevel,
		},
		User: coreuser.NameFromTag(targetUserTag),
	})
	return err
}
