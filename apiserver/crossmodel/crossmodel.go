// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
)

func init() {
	common.RegisterStandardFacadeForFeature("CrossModelRelations", 1, NewAPI, feature.CrossModelRelations)
}

// API implements the cross model interface and is the concrete
// implementation of the api end point.
type API struct {
	authorizer           facade.Authorizer
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers
	backend              Backend
	statePool            StatePool
}

// createAPI returns a new cross model API facade.
func createAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	backend Backend,
	statePool StatePool,
	authorizer facade.Authorizer,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	api := &API{
		authorizer:           authorizer,
		getApplicationOffers: getApplicationOffers,
		backend:              backend,
		statePool:            statePool,
	}
	return api, nil
}

// NewAPI returns a new cross model API facade.
func NewAPI(ctx facade.Context) (*API, error) {
	return createAPI(getApplicationOffers, getStateAccess(ctx.State()), getStatePool(ctx.StatePool()), ctx.Auth())
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *API) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	result := make([]params.ErrorResult, len(all.Offers))

	for i, one := range all.Offers {
		applicationOfferParams, err := api.makeAddOfferArgsFromParams(one)
		if err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}
		_, err = api.getApplicationOffers(api.backend).AddOffer(applicationOfferParams)
		result[i].Error = common.ServerError(err)
	}
	return params.ErrorResults{Results: result}, nil
}

func (api *API) makeAddOfferArgsFromParams(addOfferParams params.AddApplicationOffer) (jujucrossmodel.AddApplicationOfferArgs, error) {
	result := jujucrossmodel.AddApplicationOfferArgs{
		OfferName:              addOfferParams.OfferName,
		ApplicationName:        addOfferParams.ApplicationName,
		ApplicationDescription: addOfferParams.ApplicationDescription,
		Endpoints:              addOfferParams.Endpoints,
	}
	if result.OfferName == "" {
		result.OfferName = result.ApplicationName
	}
	application, err := api.backend.Application(addOfferParams.ApplicationName)
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

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *API) ApplicationOffers(urls params.ApplicationURLs) (params.ApplicationOffersResults, error) {
	var results params.ApplicationOffersResults
	results.Results = make([]params.ApplicationOfferResult, len(urls.ApplicationURLs))

	for i, urlStr := range urls.ApplicationURLs {
		offer, fullURL, err := api.offerForURL(urlStr)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = makeOfferParamsFromOffer(fullURL, offer)
	}
	return results, nil
}

// modelForName looks up the model details for the named model.
func (api *API) modelForName(modelName, userName string) (Model, bool, error) {
	user := api.authorizer.GetAuthTag().(names.UserTag)
	if userName != "" {
		user = names.NewUserTag(userName)
	}
	var model Model
	models, err := api.backend.ModelsForUser(user)
	if err != nil {
		return nil, false, err
	}
	for _, m := range models {
		if m.Model().Name() == modelName {
			model = m.Model()
			break
		}
	}
	return model, model != nil, nil
}

// offerForURL finds the single offer for a specified (possibly relative) URL,
// returning the offer and full URL.
func (api *API) offerForURL(urlStr string) (jujucrossmodel.ApplicationOffer, string, error) {
	fail := func(err error) (jujucrossmodel.ApplicationOffer, string, error) {
		return jujucrossmodel.ApplicationOffer{}, "", errors.Trace(err)
	}

	url, err := jujucrossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return fail(errors.Trace(err))
	}
	if url.Source != "" {
		err = errors.NotSupportedf("query for non-local application offers")
		return fail(errors.Trace(err))
	}

	model, ok, err := api.modelForName(url.ModelName, url.User)
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
	offers, err := api.applicationOffersFromModel(model.UUID(), filter)
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
	fullURL := fmt.Sprintf("%s/%s.%s", model.Owner().Name(), model.Name(), url.ApplicationName)
	return offers[0], fullURL, nil
}

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *API) applicationOffersFromModel(modelUUID string, filters ...jujucrossmodel.ApplicationOfferFilter) ([]jujucrossmodel.ApplicationOffer, error) {
	backend := api.backend
	if modelUUID != api.backend.ModelUUID() {
		st, releaser, err := api.statePool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		backend = st
		defer releaser()
	}

	offers, err := api.getApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return offers, nil
}

func makeOfferParamsFromOffer(url string, offer jujucrossmodel.ApplicationOffer) params.ApplicationOffer {
	result := params.ApplicationOffer{
		OfferURL:               url,
		ApplicationName:        offer.ApplicationName,
		OfferName:              offer.OfferName,
		ApplicationDescription: offer.ApplicationDescription,
	}
	for alias, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      alias,
			Interface: ep.Interface,
			Role:      ep.Role,
			Scope:     ep.Scope,
			Limit:     ep.Limit,
		})
	}
	return result
}

// FindApplicationOffers gets details about remote applications that match given filter.
func (api *API) FindApplicationOffers(filters params.OfferFilters) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults

	models := make(map[string]Model)
	filtersPerModel := make(map[string][]jujucrossmodel.ApplicationOfferFilter)

	// Group the filters per model and then query each model with the relevant filters
	// for that model.
	modelUUIDs := make(map[string]string)
	for _, f := range filters.Filters {
		// Default model is the current model.
		modelUUID := api.backend.ModelUUID()
		model, ok := models[modelUUID]
		if !ok {
			var err error
			model, err = api.backend.Model()
			if err != nil {
				return result, common.ServerError(errors.Trace(err))
			}
		}
		// If the filter contains a model name, look up the details.
		if f.ModelName != "" {
			if _, ok := modelUUIDs[f.ModelName]; !ok {
				var err error
				model, ok, err = api.modelForName(f.ModelName, "")
				if err != nil {
					return result, common.ServerError(errors.Trace(err))
				}
				if !ok {
					err := errors.NotFoundf("model %q", f.ModelName)
					return result, common.ServerError(err)
				}
				// Record the UUID for next time.
				modelUUID = model.UUID()
				modelUUIDs[f.ModelName] = modelUUID
			}
		}

		// Record the filter and model details against the model UUID.
		models[modelUUID] = model
		filters := filtersPerModel[modelUUID]
		filters = append(filters, makeOfferFilterFromParams(f))
		filtersPerModel[modelUUID] = filters
	}

	if len(filtersPerModel) == 0 {
		thisModelUUID := api.backend.ModelUUID()
		filtersPerModel[thisModelUUID] = []jujucrossmodel.ApplicationOfferFilter{}
		model, err := api.backend.Model()
		if err != nil {
			return result, common.ServerError(errors.Trace(err))
		}
		models[thisModelUUID] = model
	}

	// Do the per model queries.
	for modelUUID, filters := range filtersPerModel {
		offers, err := api.applicationOffersFromModel(modelUUID, filters...)
		if err != nil {
			return result, common.ServerError(errors.Trace(err))
		}
		model := models[modelUUID]
		baseURL := fmt.Sprintf("%s/%s", model.Owner().Name(), model.Name())

		for _, offer := range offers {
			url := fmt.Sprintf("%s.%s", baseURL, offer.OfferName)
			result.Results = append(result.Results, makeOfferParamsFromOffer(url, offer))
		}
	}
	return result, nil
}

func makeOfferFilterFromParams(filter params.OfferFilter) jujucrossmodel.ApplicationOfferFilter {
	offerFilter := jujucrossmodel.ApplicationOfferFilter{
		OfferName:              filter.OfferName,
		ApplicationName:        filter.ApplicationName,
		ApplicationDescription: filter.ApplicationDescription,
	}
	// TODO(wallyworld) - add support for Endpoint filter attribute
	return offerFilter
}
