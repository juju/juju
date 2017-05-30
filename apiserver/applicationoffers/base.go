// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
)

// BaseAPI provides various boilerplate methods used by the facade business logic.
type BaseAPI struct {
	Authorizer           facade.Authorizer
	GetApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers
	ControllerModel      Backend
	StatePool            StatePool
}

// checkPermission ensures that the logged in user holds the given permission on an entity.
func (api *BaseAPI) checkPermission(tag names.Tag, perm permission.Access) error {
	allowed, err := api.Authorizer.HasPermission(perm, tag)
	if err != nil {
		return errors.Trace(err)
	}
	if !allowed {
		return common.ErrPerm
	}
	return nil
}

// checkAdmin ensures that the logged in user is a model or controller admin.
func (api *BaseAPI) checkAdmin(backend Backend) error {
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

// modelForName looks up the model details for the named model.
func (api *BaseAPI) modelForName(modelName, ownerName string) (Model, bool, error) {
	user := api.Authorizer.GetAuthTag().(names.UserTag)
	if ownerName == "" {
		ownerName = user.Name()
	}
	var model Model
	models, err := api.ControllerModel.AllModels()
	if err != nil {
		return nil, false, err
	}
	for _, m := range models {
		if m.Name() == modelName && m.Owner().Id() == ownerName {
			model = m
			break
		}
	}
	return model, model != nil, nil
}

// applicationOffersFromModel gets details about remote applications that match given filters.
func (api *BaseAPI) applicationOffersFromModel(
	modelUUID string,
	requireAdmin bool,
	filters ...jujucrossmodel.ApplicationOfferFilter,
) ([]params.ApplicationOfferDetails, error) {
	// Get the relevant backend for the specified model.
	backend, releaser, err := api.StatePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer releaser()

	// If requireAdmin is true, the user must be a controller superuser
	// or model admin to proceed.
	isAdmin := false
	err = api.checkAdmin(backend)
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
		userAccess := permission.AdminAccess
		// If the user is not a model admin, they need at least read
		// access on an offer to see it.
		if !isAdmin {
			if userAccess, err = api.checkOfferAccess(backend, offer.OfferName, permission.ReadAccess); err != nil {
				return nil, errors.Trace(err)
			}
			if userAccess == permission.NoAccess {
				continue
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
			ApplicationOffer: makeOfferParamsFromOffer(offer, modelUUID, userAccess),
			ApplicationName:  app.Name(),
			CharmName:        curl.Name,
			ConnectedCount:   status.ConnectionCount(),
		}
		results = append(results, offerDetails)
	}
	return results, nil
}

// checkOfferAccess returns the level of access the authenticated user has to the offer,
// so long as it is greater than the requested perm.
func (api *BaseAPI) checkOfferAccess(backend Backend, offerName string, perm permission.Access) (permission.Access, error) {
	apiUser := api.Authorizer.GetAuthTag().(names.UserTag)
	access, err := backend.GetOfferAccess(names.NewApplicationOfferTag(offerName), apiUser)
	if err != nil && !errors.IsNotFound(err) {
		return permission.NoAccess, errors.Trace(err)
	}
	if !access.EqualOrGreaterOfferAccessThan(permission.ReadAccess) {
		return permission.NoAccess, nil
	}
	return access, nil
}

func makeOfferParamsFromOffer(offer jujucrossmodel.ApplicationOffer, modelUUID string, access permission.Access) params.ApplicationOffer {
	result := params.ApplicationOffer{
		SourceModelTag:         names.NewModelTag(modelUUID).String(),
		OfferName:              offer.OfferName,
		ApplicationDescription: offer.ApplicationDescription,
		Access:                 string(access),
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

type offerModel struct {
	model Model
	err   error
}

// getModelsFromOffers returns a slice of models corresponding to the
// specified offer URLs. Each result item has either a model or an error.
func (api *BaseAPI) getModelsFromOffers(offerURLs ...string) ([]offerModel, error) {
	// Cache the models found so far so we don't look them up more than once.
	modelsCache := make(map[string]Model)
	oneModel := func(offerURL string) (Model, error) {
		url, err := jujucrossmodel.ParseApplicationURL(offerURL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelPath := fmt.Sprintf("%s/%s", url.User, url.ModelName)
		if model, ok := modelsCache[modelPath]; ok {
			return model, nil
		}

		model, ok, err := api.modelForName(url.ModelName, url.User)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !ok {
			return nil, errors.NotFoundf("model %q", modelPath)
		}
		return model, nil
	}

	result := make([]offerModel, len(offerURLs))
	for i, offerURL := range offerURLs {
		var om offerModel
		om.model, om.err = oneModel(offerURL)
		result[i] = om
	}
	return result, nil
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
		if f.ModelName == "" {
			return nil, nil, errors.New("application offer filter must specify a model name")
		}
		var (
			modelUUID string
			ok        bool
		)
		if modelUUID, ok = modelUUIDs[f.ModelName]; !ok {
			var err error
			model, ok, err := api.modelForName(f.ModelName, f.OwnerName)
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

		// Record the filter and model details against the model UUID.
		filters := filtersPerModel[modelUUID]
		filters = append(filters, makeOfferFilterFromParams(f))
		filtersPerModel[modelUUID] = filters
	}
	return models, filtersPerModel, nil
}

// getApplicationOffersDetails gets details about remote applications that match given filter.
func (api *BaseAPI) getApplicationOffersDetails(
	filters params.OfferFilters,
	requireAdmin bool,
) ([]params.ApplicationOfferDetails, error) {

	// If there are no filters specified, that's an error since the
	// caller is expected to specify at the least one or more models
	// to avoid an unbounded query across all models.
	if len(filters.Filters) == 0 {
		return nil, common.ServerError(errors.New("at least one offer filter is required"))
	}

	// Gather all the filter details for doing a query for each model.
	models, filtersPerModel, err := api.getModelFilters(filters)
	if err != nil {
		return nil, common.ServerError(errors.Trace(err))
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
		offers, err := api.applicationOffersFromModel(modelUUID, requireAdmin, filters...)
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
