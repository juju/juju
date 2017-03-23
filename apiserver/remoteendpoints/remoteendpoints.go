// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoteendpoints

import (
	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodelcommon"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/permission"
)

func init() {
	common.RegisterStandardFacadeForFeature("RemoteEndpoints", 1, NewEndpointsAPI, feature.CrossModelRelations)
}

// EndpointsAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type EndpointsAPI struct {
	crossmodelcommon.BaseAPI
}

// createEndpointsAPI returns a new EndpointsAPI facade.
func createEndpointsAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	backend crossmodelcommon.Backend,
	statePool crossmodelcommon.StatePool,
	authorizer facade.Authorizer,
) (*EndpointsAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &EndpointsAPI{
		BaseAPI: crossmodelcommon.BaseAPI{
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			Backend:              backend,
			StatePool:            statePool,
		}}
	return api, nil
}

// NewEndpointsAPI returns a new EndpointsAPI facade.
func NewEndpointsAPI(ctx facade.Context) (*EndpointsAPI, error) {
	return createEndpointsAPI(
		crossmodelcommon.GetApplicationOffers, crossmodelcommon.GetStateAccess(ctx.State()),
		crossmodelcommon.GetStatePool(ctx.StatePool()), ctx.Auth())
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *EndpointsAPI) ApplicationOffers(urls params.ApplicationURLs) (params.ApplicationOffersResults, error) {
	var results params.ApplicationOffersResults
	results.Results = make([]params.ApplicationOfferResult, len(urls.ApplicationURLs))

	for i, urlStr := range urls.ApplicationURLs {
		offer, err := api.offerForURL(urlStr)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = offer.ApplicationOffer
	}
	return results, nil
}

// offerForURL finds the single offer for a specified (possibly relative) URL,
// returning the offer and full URL.
func (api *EndpointsAPI) offerForURL(urlStr string) (params.ApplicationOfferDetails, error) {
	fail := func(err error) (params.ApplicationOfferDetails, error) {
		return params.ApplicationOfferDetails{}, errors.Trace(err)
	}

	url, err := jujucrossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if url.Source != "" {
		err = errors.NotSupportedf("query for non-local application offers")
		return fail(errors.Trace(err))
	}

	model, ok, err := api.ModelForName(url.ModelName, url.User)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if !ok {
		err = errors.NotFoundf("model %q", url.ModelName)
		return fail(err)
	}
	filter := jujucrossmodel.ApplicationOfferFilter{
		OfferName: url.ApplicationName,
	}
	offers, err := api.ApplicationOffersFromModel(model.UUID(), permission.ReadAccess, filter)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if len(offers) == 0 {
		err := errors.NotFoundf("application offer %q", url.ApplicationName)
		return fail(err)
	}
	if len(offers) > 1 {
		err := errors.Errorf("too many application offers for %q", url.ApplicationName)
		return fail(err)
	}
	fullURL := jujucrossmodel.MakeURL(model.Owner().Name(), model.Name(), url.ApplicationName, "")
	offer := offers[0]
	offer.OfferURL = fullURL
	return offer, nil
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *EndpointsAPI) FindApplicationOffers(filters params.OfferFilters) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults
	var filtersToUse params.OfferFilters

	// If there is only one filter term, and no model is specified, add in
	// any models the user can see and query across those.
	// If there's more than one filter term, each must specify a model.
	if len(filters.Filters) == 1 && filters.Filters[0].ModelName == "" {
		user := api.Authorizer.GetAuthTag().(names.UserTag)
		userModels, err := api.Backend.ModelsForUser(user)
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, um := range userModels {
			modelFilter := filters.Filters[0]
			modelFilter.ModelName = um.Model().Name()
			modelFilter.OwnerName = um.Model().Owner().Name()
			filtersToUse.Filters = append(filtersToUse.Filters, modelFilter)
		}
	} else {
		filtersToUse = filters
	}
	for _, f := range filtersToUse.Filters {
		if f.ModelName == "" {
			return result, errors.New("application offer filter must specify a model name")
		}
	}

	offers, err := api.GetApplicationOffersDetails(filtersToUse, permission.ReadAccess)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, offer := range offers {
		result.Results = append(result.Results, offer.ApplicationOffer)
	}
	return result, nil
}
