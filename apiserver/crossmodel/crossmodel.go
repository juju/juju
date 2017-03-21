// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package crossmodel provides an API server facade for managing
// cross model relations.
package crossmodel

import (
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/permission"
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

func (api *API) checkPermission(backend Backend, perm permission.Access) error {
	allowed, err := api.authorizer.HasPermission(perm, api.backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

// Offer makes application endpoints available for consumption at a specified URL.
func (api *API) Offer(all params.AddApplicationOffers) (params.ErrorResults, error) {
	if err := api.checkPermission(api.backend, permission.AdminAccess); err != nil {
		return params.ErrorResults{}, common.ServerError(err)
	}

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
		offer, err := api.offerForURL(urlStr)
		if err != nil {
			results.Results[i].Error = common.ServerError(err)
			continue
		}
		results.Results[i].Result = offer.ApplicationOffer
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
func (api *API) offerForURL(urlStr string) (params.ApplicationOfferDetails, error) {
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
	offers, err := api.applicationOffersFromModel(model.UUID(), permission.ReadAccess, filter)
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

// ApplicationOffers gets details about remote applications that match given URLs.
func (api *API) applicationOffersFromModel(
	modelUUID string,
	requiredPerm permission.Access,
	filters ...jujucrossmodel.ApplicationOfferFilter,
) ([]params.ApplicationOfferDetails, error) {
	backend := api.backend
	if modelUUID != api.backend.ModelUUID() {
		st, releaser, err := api.statePool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		backend = st
		defer releaser()
	}
	if err := api.checkPermission(backend, requiredPerm); err != nil {
		return nil, common.ServerError(err)
	}

	offers, err := api.getApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferDetails
	for _, offer := range offers {
		app, err := backend.Application(offer.ApplicationName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		curl, _ := app.CharmURL()
		status, err := backend.RemoteConnectionStatus(offer.OfferName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		offerDetails := params.ApplicationOfferDetails{
			ApplicationOffer: makeOfferParamsFromOffer(offer),
			ApplicationName:  app.Name(),
			CharmName:        curl.Name,
			ConnectedCount:   status.ConnectionCount(),
		}
		results = append(results, offerDetails)
	}
	return results, nil
}

func makeOfferParamsFromOffer(offer jujucrossmodel.ApplicationOffer) params.ApplicationOffer {
	result := params.ApplicationOffer{
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

// FindApplicationOffers gets information about remote applications that match given filter.
// The result describes the endpoints able to be used in a remote relation.
func (api *API) FindApplicationOffers(filters params.OfferFilters) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults
	offers, err := api.getApplicationOffersDetails(filters, permission.ReadAccess)
	if err != nil {
		return result, err
	}
	for _, offer := range offers {
		result.Results = append(result.Results, offer.ApplicationOffer)
	}
	return result, nil
}

// ListApplicationOffers gets deployed details about application offers that match given filter.
// The results contain details about the deployed applications such as connection count.
// The results contain sensitive data about the state of the model, restricted to model admins.
func (api *API) ListApplicationOffers(filters params.OfferFilters) (params.ListApplicationOffersResults, error) {
	var result params.ListApplicationOffersResults
	offers, err := api.getApplicationOffersDetails(filters, permission.AdminAccess)
	if err != nil {
		return result, err
	}
	result.Results = offers
	return result, nil
}

// getModelFilters splits the specified filters per model and returns
// the model and filter details for each.
func (api *API) getModelFilters(filters params.OfferFilters) (
	models map[string]Model,
	filtersPerModel map[string][]jujucrossmodel.ApplicationOfferFilter,
	_ error,
) {
	models = make(map[string]Model)
	filtersPerModel = make(map[string][]jujucrossmodel.ApplicationOfferFilter)

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
				return nil, nil, errors.Trace(err)
			}
			models[modelUUID] = model
		}
		// If the filter contains a model name, look up the details.
		if f.ModelName != "" {
			if modelUUID, ok = modelUUIDs[f.ModelName]; !ok {
				var err error
				model, ok, err := api.modelForName(f.ModelName, "")
				if err != nil {
					return nil, nil, errors.Trace(err)
				}
				if !ok {
					err := errors.NotFoundf("model %q", f.ModelName)
					return nil, nil, errors.Trace(err)
				}
				// Record the UUID and model for next time.
				modelUUID = model.UUID()
				modelUUIDs[f.ModelName] = modelUUID
				models[modelUUID] = model
			}
		}

		// Record the filter and model details against the model UUID.
		filters := filtersPerModel[modelUUID]
		filters = append(filters, makeOfferFilterFromParams(f))
		filtersPerModel[modelUUID] = filters
	}
	return models, filtersPerModel, nil
}

// getApplicationOffersDetails gets details about remote applications that match given filter.
func (api *API) getApplicationOffersDetails(
	filters params.OfferFilters,
	requiredPerm permission.Access,
) ([]params.ApplicationOfferDetails, error) {
	// Gather all the filter details for doing a query for each model.
	models, filtersPerModel, err := api.getModelFilters(filters)
	if err != nil {
		return nil, common.ServerError(errors.Trace(err))
	}

	if len(filtersPerModel) == 0 {
		thisModelUUID := api.backend.ModelUUID()
		filtersPerModel[thisModelUUID] = []jujucrossmodel.ApplicationOfferFilter{}
		model, err := api.backend.Model()
		if err != nil {
			return nil, common.ServerError(errors.Trace(err))
		}
		models[thisModelUUID] = model
	}

	// Ensure the result is deterministic.
	var allUUIDs []string
	for modelUUID := range filtersPerModel {
		allUUIDs = append(allUUIDs, modelUUID)
	}
	sort.Strings(allUUIDs)

	// Do the per model queries.
	var result []params.ApplicationOfferDetails
	for _, modelUUID := range allUUIDs {
		filters := filtersPerModel[modelUUID]
		offers, err := api.applicationOffersFromModel(modelUUID, requiredPerm, filters...)
		if err != nil {
			return nil, common.ServerError(errors.Trace(err))
		}
		model := models[modelUUID]

		for _, offer := range offers {
			offer.OfferURL = jujucrossmodel.MakeURL(model.Owner().Name(), model.Name(), offer.OfferName, "")
			result = append(result, offer)
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
