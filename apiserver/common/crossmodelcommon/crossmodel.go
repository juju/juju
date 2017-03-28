// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package crossmodelcommon

import (
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

// BaseAPI provides common facade functionality for cross model relations related facades.
type BaseAPI struct {
	Authorizer           facade.Authorizer
	GetApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers
	Backend              Backend
	StatePool            StatePool
}

// CheckPermission ensures that the logged in user holds the given permission on an entity.
func (api *BaseAPI) CheckPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.Authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

// CheckAdmin ensures that the logged in user is a model or controller admin.
func (api *BaseAPI) CheckAdmin(backend Backend) error {
	allowed, err := api.Authorizer.HasPermission(permission.AdminAccess, backend.ModelTag())
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		allowed, err = api.Authorizer.HasPermission(permission.SuperuserAccess, backend.ControllerTag())
	}
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

// ModelForName looks up the model details for the named model.
func (api *BaseAPI) ModelForName(modelName, ownerName string) (Model, bool, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	if ownerName == "" {
		ownerName = user.Name()
	}
	var model Model
	models, err := api.Backend.AllModels()
	if err != nil {
		return nil, false, err
	}
	for _, m := range models {
		if m.Name() == modelName && m.Owner().Name() == ownerName {
			model = m
			break
		}
	}
	return model, model != nil, nil
}

// ApplicationOffersFromModel gets details about remote applications that match given filters.
func (api *BaseAPI) ApplicationOffersFromModel(
	modelUUID string,
	requireAdmin bool,
	filters ...jujucrossmodel.ApplicationOfferFilter,
) ([]params.ApplicationOfferDetails, error) {
	backend := api.Backend
	if modelUUID != api.Backend.ModelUUID() {
		st, releaser, err := api.StatePool.Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		backend = st
		defer releaser()
	}
	// If requireAdmin is true, the user must be a controller superuser
	// or model admin to proceed.
	isAdmin := false
	err := api.CheckAdmin(backend)
	if err != nil && err != common.ErrPerm {
		return nil, errors.Trace(err)
	}
	isAdmin = err == nil
	if requireAdmin && !isAdmin {
		return nil, common.ServerError(err)
	}

	offers, err := api.GetApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferDetails
	for _, offer := range offers {
		// If the user is not a model admin, they need at least read
		// access on an offer to see it.
		if !isAdmin {
			permErr := api.CheckPermission(names.NewApplicationOfferTag(offer.OfferName), permission.ReadAccess)
			if permErr == common.ErrPerm {
				continue
			}
			if permErr != nil {
				return nil, errors.Trace(permErr)
			}
		}
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

// getModelFilters splits the specified filters per model and returns
// the model and filter details for each.
func (api *BaseAPI) getModelFilters(filters params.OfferFilters) (
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
		modelUUID := api.Backend.ModelUUID()
		model, ok := models[modelUUID]
		if !ok {
			var err error
			model, err = api.Backend.Model()
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			models[modelUUID] = model
		}
		// If the filter contains a model name, look up the details.
		if f.ModelName != "" {
			if modelUUID, ok = modelUUIDs[f.ModelName]; !ok {
				var err error
				model, ok, err := api.ModelForName(f.ModelName, f.OwnerName)
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

// GetApplicationOffersDetails gets details about remote applications that match given filter.
func (api *BaseAPI) GetApplicationOffersDetails(
	filters params.OfferFilters,
	requireAdmin bool,
) ([]params.ApplicationOfferDetails, error) {
	// Gather all the filter details for doing a query for each model.
	models, filtersPerModel, err := api.getModelFilters(filters)
	if err != nil {
		return nil, common.ServerError(errors.Trace(err))
	}

	if len(filtersPerModel) == 0 {
		thisModelUUID := api.Backend.ModelUUID()
		filtersPerModel[thisModelUUID] = []jujucrossmodel.ApplicationOfferFilter{}
		model, err := api.Backend.Model()
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
		offers, err := api.ApplicationOffersFromModel(modelUUID, requireAdmin, filters...)
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
