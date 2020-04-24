// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"github.com/juju/txn"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
)

var logger = loggo.GetLogger("juju.apiserver.applicationoffers")

type environFromModelFunc func(string) (environs.Environ, error)

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	BaseAPI
	dataDir     string
	authContext *commoncrossmodel.AuthContext
}

// OffersAPIV2 implements the cross model interface V2.
type OffersAPIV2 struct {
	*OffersAPI
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	getEnviron environFromModelFunc,
	getControllerInfo func() ([]string, string, error),
	backend Backend,
	statePool StatePool,
	authorizer facade.Authorizer,
	resources facade.Resources,
	authContext *commoncrossmodel.AuthContext,
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	dataDir := resources.Get("dataDir").(common.StringResource)
	api := &OffersAPI{
		dataDir:     dataDir.String(),
		authContext: authContext,
		BaseAPI: BaseAPI{
			ctx:                  context.Background(),
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			ControllerModel:      backend,
			StatePool:            statePool,
			getEnviron:           getEnviron,
			getControllerInfo:    getControllerInfo,
		},
	}
	return api, nil
}

// NewOffersAPIV2 returns a new application offers OffersAPIV2 facade.
func NewOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	environFromModel := func(modelUUID string) (environs.Environ, error) {
		st, err := ctx.StatePool().Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		g := stateenvirons.EnvironConfigGetter{Model: model}
		env, err := environs.GetEnviron(g, environs.New)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return env, nil
	}

	st := ctx.State()
	getControllerInfo := func() ([]string, string, error) {
		return common.StateControllerInfo(st)
	}

	authContext := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value
	return createOffersAPI(
		GetApplicationOffers,
		environFromModel,
		getControllerInfo,
		GetStateAccess(st),
		GetStatePool(ctx.StatePool()),
		ctx.Auth(),
		ctx.Resources(),
		authContext.(*commoncrossmodel.AuthContext),
	)
}

// NewOffersAPIV2 returns a new application offers OffersAPIV2 facade.
func NewOffersAPIV2(ctx facade.Context) (*OffersAPIV2, error) {
	apiV1, err := NewOffersAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &OffersAPIV2{OffersAPI: apiV1}, nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *OffersAPI) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))

	for i, one := range all.Offers {
		modelTag, err := names.ParseModelTag(one.ModelTag)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		backend, releaser, err := api.StatePool.Get(modelTag.Id())
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		defer releaser()

		if err := api.checkAdmin(backend); err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}

		applicationOfferParams, err := api.makeAddOfferArgsFromParams(backend, one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		_, err = api.GetApplicationOffers(backend).AddOffer(applicationOfferParams)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *OffersAPI) makeAddOfferArgsFromParams(backend Backend, addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
		Owner:                  api.Authorizer.GetAuthTag().Id(),
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
func (api *OffersAPI) ListApplicationOffers(filters params.OfferFilters) (params.QueryApplicationOffersResults, error) {
	var result params.QueryApplicationOffersResults
	offers, err := api.getApplicationOffersDetails(filters, permission.AdminAccess)
	if err != nil {
		return result, common.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// ModifyOfferAccess changes the application offer access granted to users.
func (api *OffersAPI) ModifyOfferAccess(args params.ModifyOfferAccessRequest) (result params.ErrorResults, _ error) {
	result = params.ErrorResults{
		Results: make([]params.ErrorResult, len(args.Changes)),
	}
	if len(args.Changes) == 0 {
		return result, nil
	}

	isControllerAdmin, err := api.Authorizer.HasPermission(permission.SuperuserAccess, api.ControllerModel.ControllerTag())
	if err != nil {
		return result, errors.Trace(err)
	}

	offerURLs := make([]string, len(args.Changes))
	for i, arg := range args.Changes {
		offerURLs[i] = arg.OfferURL
	}
	models, err := api.getModelsFromOffers(offerURLs...)
	if err != nil {
		return result, errors.Trace(err)
	}

	for i, arg := range args.Changes {
		if models[i].err != nil {
			result.Results[i].Error = common.ServerError(models[i].err)
			continue
		}
		err = api.modifyOneOfferAccess(models[i].model.UUID(), isControllerAdmin, arg)
		result.Results[i].Error = common.ServerError(err)
	}
	return result, nil
}

func (api *OffersAPI) modifyOneOfferAccess(modelUUID string, isControllerAdmin bool, arg params.ModifyOfferAccess) error {
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
		if canModifyOffer, err = api.Authorizer.HasPermission(permission.AdminAccess, backend.ModelTag()); err != nil {
			return errors.Trace(err)
		}
	}

	if !canModifyOffer {
		apiUser := api.Authorizer.GetAuthTag().(names.UserTag)
		offer, err := backend.ApplicationOffer(offerTag.Id())
		if err != nil {
			return common.ErrPerm
		}
		access, err := backend.GetOfferAccess(offer.OfferUUID, apiUser)
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		} else if err == nil {
			canModifyOffer = access == permission.AdminAccess
		}
	}
	if !canModifyOffer {
		return common.ErrPerm
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
			return common.ErrPerm
		}
		offerAccess, err := backend.GetOfferAccess(offer.OfferUUID, targetUserTag)
		if errors.IsNotFound(err) {
			// Conflicts with prior check, must be inconsistent state.
			err = txn.ErrExcessiveContention
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
func (api *OffersAPI) ApplicationOffers(urls params.OfferURLs) (params.ApplicationOffersResults, error) {
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
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		if url.User == "" {
			url.User = api.Authorizer.GetAuthTag().Id()
		}
		if url.HasEndpoint() {
			results.Results[i].Error = common.ServerError(
				errors.Errorf("remote application %q shouldn't include endpoint", url))
			continue
		}
		if url.Source != "" {
			results.Results[i].Error = common.ServerError(
				errors.NotSupportedf("query for non-local application offers"))
			continue
		}
		fullURLs = append(fullURLs, url.String())
		filters = append(filters, api.filterFromURL(url))
	}
	if len(filters) == 0 {
		return results, nil
	}
	offers, err := api.getApplicationOffersDetails(params.OfferFilters{filters}, permission.ReadAccess)
	if err != nil {
		return results, common.ServerError(err)
	}
	offersByURL := make(map[string]params.ApplicationOfferAdminDetails)
	for _, offer := range offers {
		offersByURL[offer.OfferURL] = offer
	}

	for i, urlStr := range fullURLs {
		offer, ok := offersByURL[urlStr]
		if !ok {
			err = errors.NotFoundf("application offer %q", urlStr)
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = &offer
	}
	return results, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *OffersAPI) FindApplicationOffers(filters params.OfferFilters) (params.QueryApplicationOffersResults, error) {
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
			modelFilter.OwnerName = m.Owner().Name()
			filtersToUse.Filters = append(filtersToUse.Filters, modelFilter)
		}
	} else {
		filtersToUse = filters
	}
	offers, err := api.getApplicationOffersDetails(filtersToUse, permission.ReadAccess)
	if err != nil {
		return result, common.ServerError(err)
	}
	result.Results = offers
	return result, nil
}

// GetConsumeDetails returns the details necessary to pass to another model to
// consume the specified offers represented by the urls.
func (api *OffersAPI) GetConsumeDetails(args params.OfferURLs) (params.ConsumeOfferDetailsResults, error) {
	var consumeResults params.ConsumeOfferDetailsResults
	results := make([]params.ConsumeOfferDetailsResult, len(args.OfferURLs))

	offers, err := api.ApplicationOffers(args)
	if err != nil {
		return consumeResults, common.ServerError(err)
	}

	addrs, caCert, err := api.getControllerInfo()
	if err != nil {
		return consumeResults, common.ServerError(err)
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
		offerMacaroon, err := api.authContext.CreateConsumeOfferMacaroon(api.ctx, offerDetails, api.Authorizer.GetAuthTag().Id(), args.BakeryVersion)
		if err != nil {
			results[i].Error = common.ServerError(err)
			continue
		}
		results[i].Macaroon = offerMacaroon.M()
	}
	consumeResults.Results = results
	return consumeResults, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
// This call currently has no client side API, only there for the GUI at this stage.
func (api *OffersAPI) RemoteApplicationInfo(args params.OfferURLs) (params.RemoteApplicationInfoResults, error) {
	results := make([]params.RemoteApplicationInfoResult, len(args.OfferURLs))
	for i, url := range args.OfferURLs {
		info, err := api.oneRemoteApplicationInfo(url)
		results[i].Result = info
		results[i].Error = common.ServerError(err)
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

func (api *OffersAPI) oneRemoteApplicationInfo(urlStr string) (*params.RemoteApplicationInfo, error) {
	url, err := jujucrossmodel.ParseOfferURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need at least read access to the model to see the application details.
	// 	offer, err := api.offeredApplicationDetails(url, permission.ReadAccess)
	offers, err := api.getApplicationOffersDetails(
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

// DestroyOffers removes the offers specified by the given URLs.
func (api *OffersAPI) DestroyOffers(args params.DestroyApplicationOffers) (params.ErrorResults, error) {
	return destroyOffers(api, args.OfferURLs, false)
}

// DestroyOffers removes the offers specified by the given URLs, forcing if necessary.
func (api *OffersAPIV2) DestroyOffers(args params.DestroyApplicationOffers) (params.ErrorResults, error) {
	return destroyOffers(api.OffersAPI, args.OfferURLs, args.Force)
}

func destroyOffers(api *OffersAPI, offerURLs []string, force bool) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(offerURLs))

	models, err := api.getModelsFromOffers(offerURLs...)
	if err != nil {
		return params.ErrorResults{}, errors.Trace(err)
	}

	for i, one := range offerURLs {
		url, err := jujucrossmodel.ParseOfferURL(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		if models[i].err != nil {
			result[i].Error = common.ServerError(models[i].err)
			continue
		}
		backend, releaser, err := api.StatePool.Get(models[i].model.UUID())
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		defer releaser()

		if err := api.checkAdmin(backend); err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		err = api.GetApplicationOffers(backend).Remove(url.ApplicationName, force)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}
