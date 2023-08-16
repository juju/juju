// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	jujutxn "github.com/juju/txn/v3"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/rpc/params"
)

type environFromModelFunc func(string) (environs.Environ, error)

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	BaseAPI
	dataDir     string
	authContext *commoncrossmodel.AuthContext
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	getEnviron environFromModelFunc,
	getControllerInfo func() ([]string, string, error),
	backend Backend,
	statePool StatePool,
	authorizer facade.Authorizer,
	authContext *commoncrossmodel.AuthContext,
	dataDir string,
	logger loggo.Logger,
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	api := &OffersAPI{
		dataDir:     dataDir,
		authContext: authContext,
		BaseAPI: BaseAPI{
			ctx:                  context.Background(),
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			ControllerModel:      backend,
			StatePool:            statePool,
			getEnviron:           getEnviron,
			getControllerInfo:    getControllerInfo,
			logger:               logger,
		},
	}
	return api, nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPI) Offer(ctx context.Context, all params.AddApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))

	apiUser := api.Authorizer.GetAuthTag().(names.UserTag)
	for i, one := range all.Offers {
		modelTag, err := names.ParseModelTag(one.ModelTag)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		backend, releaser, err := api.StatePool.Get(modelTag.Id())
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		defer releaser()

		if err := api.checkAdmin(apiUser, backend); err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}

		owner := apiUser
		// The V4 version of the api includes the offer owner in the params.
		if one.OwnerTag != "" {
			var err error
			if owner, err = names.ParseUserTag(one.OwnerTag); err != nil {
				result[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}
		applicationOfferParams, err := api.makeAddOfferArgsFromParams(owner, backend, one)
		if err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}

		offerBackend := api.GetApplicationOffers(backend)
		if _, err = offerBackend.ApplicationOffer(applicationOfferParams.OfferName); err == nil {
			_, err = offerBackend.UpdateOffer(applicationOfferParams)
		} else {
			_, err = offerBackend.AddOffer(applicationOfferParams)
		}
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPI) makeAddOfferArgsFromParams(user names.UserTag, backend Backend, addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
		Owner:                  user.Id(),
		HasRead:                []string{common.EveryoneTagName},
	}
	if result.OfferName == "" {
		result.OfferName = result.ApplicationName
	}
	application, err := backend.Application(addOfferParams.ApplicationName)
	if err != nil {
		return result, errors.Annotatef(err, "getting offered application %v", addOfferParams.ApplicationName)
	}

	if result.ApplicationDescription == "" {
		ch, _, err := application.Charm()
		if err != nil {
			return result,
				errors.Annotatef(err, "getting charm for application %v", addOfferParams.ApplicationName)
		}
		result.ApplicationDescription = ch.Meta().Description
	}
	return result, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
func (api *OffersAPI) ListApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResults, error) {
	var result params.QueryApplicationOffersResults
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	offers, err := api.getApplicationOffersDetails(user, filters, permission.AdminAccess)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPI) ModifyOfferAccess(ctx context.Context, args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	err := api.Authorizer.HasPermission(permission.SuperuserAccess, api.ControllerModel.ControllerTag())
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
		err = api.modifyOneOfferAccess(user, models[i].model.UUID(), isControllerAdmin, arg)
		result.Results[i].Error = apiservererrors.ServerError(err)
	}
	return result, nil
}

func (api *OffersAPI) modifyOneOfferAccess(user names.UserTag, modelUUID string, isControllerAdmin bool, arg params.ModifyOfferAccess) error {
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
	offerTag := names.NewApplicationOfferTag(url.ApplicationName)

	canModifyOffer := isControllerAdmin
	if !canModifyOffer {
		err = api.Authorizer.HasPermission(permission.AdminAccess, backend.ModelTag())
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return errors.Trace(err)
		}
	}

	if !canModifyOffer {
		offer, err := backend.ApplicationOffer(offerTag.Id())
		if err != nil {
			return apiservererrors.ErrPerm
		}
		access, err := backend.GetOfferAccess(offer.OfferUUID, user)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		} else if err == nil {
			canModifyOffer = access == permission.AdminAccess
		}
	}
	if !canModifyOffer {
		return apiservererrors.ErrPerm
	}

	targetUserTag, err := names.ParseUserTag(arg.UserTag)
	if err != nil {
		return errors.Annotate(err, "could not modify offer access")
	}
	return api.changeOfferAccess(backend, offerTag, targetUserTag, arg.Action, offerAccess)
}

// changeOfferAccess performs the requested access grant or revoke action for the
// specified user on the specified application offer.
func (api *OffersAPI) changeOfferAccess(
	backend Backend,
	offerTag names.ApplicationOfferTag,
	targetUserTag names.UserTag,
	action params.OfferAction,
	access permission.Access,
) error {
	_, err := backend.ApplicationOffer(offerTag.Name)
	if err != nil {
		return errors.Trace(err)
	}
	switch action {
	case params.GrantOfferAccess:
		return api.grantOfferAccess(backend, offerTag, targetUserTag, access)
	case params.RevokeOfferAccess:
		return api.revokeOfferAccess(backend, offerTag, targetUserTag, access)
	default:
		return errors.Errorf("unknown action %q", action)
	}
}

func (api *OffersAPI) grantOfferAccess(backend Backend, offerTag names.ApplicationOfferTag, targetUserTag names.UserTag, access permission.Access) error {
	err := backend.CreateOfferAccess(offerTag, targetUserTag, access)
	if errors.IsAlreadyExists(err) {
		offer, err := backend.ApplicationOffer(offerTag.Id())
		if err != nil {
			return apiservererrors.ErrPerm
		}
		offerAccess, err := backend.GetOfferAccess(offer.OfferUUID, targetUserTag)
		if errors.IsNotFound(err) {
			// Conflicts with prior check, must be inconsistent state.
			err = jujutxn.ErrExcessiveContention
		}
		if err != nil {
			return errors.Annotate(err, "could not look up offer access for user")
		}

		// Only set access if greater access is being granted.
		if offerAccess.EqualOrGreaterOfferAccessThan(access) {
			return errors.Errorf("user already has %q access or greater", access)
		}
		if err = backend.UpdateOfferAccess(offerTag, targetUserTag, access); err != nil {
			return errors.Annotate(err, "could not set offer access for user")
		}
		return nil
	}
	return errors.Annotate(err, "could not grant offer access")
}

func (api *OffersAPI) revokeOfferAccess(backend Backend, offerTag names.ApplicationOfferTag, targetUserTag names.UserTag, access permission.Access) error {
	switch access {
	case permission.ReadAccess:
		// Revoking read access removes all access.
		err := backend.RemoveOfferAccess(offerTag, targetUserTag)
		return errors.Annotate(err, "could not revoke offer access")
	case permission.ConsumeAccess:
		// Revoking consume access sets read-only.
		err := backend.UpdateOfferAccess(offerTag, targetUserTag, permission.ReadAccess)
		return errors.Annotate(err, "could not set offer access to read-only")
	case permission.AdminAccess:
		// Revoking admin access sets read-consume.
		err := backend.UpdateOfferAccess(offerTag, targetUserTag, permission.ConsumeAccess)
		return errors.Annotate(err, "could not set offer access to read-consume")

	default:
		return errors.Errorf("don't know how to revoke %q access", access)
	}
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *OffersAPI) ApplicationOffers(ctx context.Context, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	return api.getApplicationOffers(user, urls)
}

func (api *OffersAPI) getApplicationOffers(user names.UserTag, urls params.OfferURLs) (params.ApplicationOffersResults, error) {
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
	offers, err := api.getApplicationOffersDetails(user, params.OfferFilters{filters}, permission.ReadAccess)
	if err != nil {
		return results, apiservererrors.ServerError(err)
	}
	offersByURL := make(map[string]params.ApplicationOfferAdminDetails)
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
func (api *OffersAPI) FindApplicationOffers(ctx context.Context, filters params.OfferFilters) (params.QueryApplicationOffersResults, error) {
	var result params.QueryApplicationOffersResults
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
	offers, err := api.getApplicationOffersDetails(user, filtersToUse, permission.ReadAccess)
	if err != nil {
		return result, apiservererrors.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// GetConsumeDetails returns the details necessary to pass to another model
// to allow the specified args user to consume the offers represented by the args URLs.
func (api *OffersAPI) GetConsumeDetails(args params.ConsumeOfferDetailsArg) (params.ConsumeOfferDetailsResults, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	// Prefer args user if provided.
	if args.UserTag != "" {
		// Only controller admins can get consume details for another user.
		err := api.checkControllerAdmin()
		if err != nil {
			return params.ConsumeOfferDetailsResults{}, errors.Trace(err)
		}
		user, err = names.ParseUserTag(args.UserTag)
		if err != nil {
			return params.ConsumeOfferDetailsResults{}, errors.Trace(err)
		}
	}
	return api.getConsumeDetails(user, args.OfferURLs)
}

// getConsumeDetails returns the details necessary to pass to another model to
// to allow the specified user to consume the specified offers represented by the urls.
func (api *OffersAPI) getConsumeDetails(user names.UserTag, urls params.OfferURLs) (params.ConsumeOfferDetailsResults, error) {
	var consumeResults params.ConsumeOfferDetailsResults
	results := make([]params.ConsumeOfferDetailsResult, len(urls.OfferURLs))

	offers, err := api.getApplicationOffers(user, urls)
	if err != nil {
		return consumeResults, apiservererrors.ServerError(err)
	}

	addrs, caCert, err := api.getControllerInfo()
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
		offerDetails := &offer.ApplicationOfferDetails
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

		err = api.checkAdmin(user, backend)
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			results[i].Error = apiservererrors.ServerError(err)
			continue
		}
		isAdmin := err == nil
		if !isAdmin {
			appOffer := names.NewApplicationOfferTag(offer.OfferUUID)
			err := api.Authorizer.EntityHasPermission(user, permission.ConsumeAccess, appOffer)
			if err != nil {
				results[i].Error = apiservererrors.ServerError(err)
				continue
			}
		}

		offerMacaroon, err := api.authContext.CreateConsumeOfferMacaroon(api.ctx, offerDetails, user.Id(), urls.BakeryVersion)
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
func (api *OffersAPI) RemoteApplicationInfo(ctx context.Context, args params.OfferURLs) (params.RemoteApplicationInfoResults, error) {
	results := make([]params.RemoteApplicationInfoResult, len(args.OfferURLs))
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	for i, url := range args.OfferURLs {
		info, err := api.oneRemoteApplicationInfo(user, url)
		results[i].Result = info
		results[i].Error = apiservererrors.ServerError(err)
	}
	return params.RemoteApplicationInfoResults{results}, nil
}

func (api *OffersAPI) filterFromURL(url *jujucrossmodel.OfferURL) params.OfferFilter {
	f := params.OfferFilter{
		OwnerName: url.User,
		ModelName: url.ModelName,
		OfferName: url.ApplicationName,
	}
	return f
}

func (api *OffersAPI) oneRemoteApplicationInfo(user names.UserTag, urlStr string) (*params.RemoteApplicationInfo, error) {
	url, err := jujucrossmodel.ParseOfferURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need at least read access to the model to see the application details.
	// 	offer, err := api.offeredApplicationDetails(url, permission.ReadAccess)
	offers, err := api.getApplicationOffersDetails(
		user,
		params.OfferFilters{[]params.OfferFilter{api.filterFromURL(url)}}, permission.ConsumeAccess)
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
func (api *OffersAPI) DestroyOffers(ctx context.Context, args params.DestroyApplicationOffers) (params.ErrorResults, error) {
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

		if err := api.checkAdmin(user, backend); err != nil {
			result[i].Error = apiservererrors.ServerError(err)
			continue
		}
		err = api.GetApplicationOffers(backend).Remove(url.ApplicationName, args.Force)
		result[i].Error = apiservererrors.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}
