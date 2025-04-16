// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

// BaseAPI provides various boilerplate methods used by the facade business logic.
type BaseAPI struct {
	Authorizer           facade.Authorizer
	GetApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers
	ControllerModel      Backend
	StatePool            StatePool
	getEnviron           environFromModelFunc
	getControllerInfo    func() (apiAddrs []string, caCert string, _ error)
	ctx                  context.Context
}

// checkAdmin ensures that the specified in user is a model or controller admin.
func (api *BaseAPI) checkAdmin(user names.UserTag, backend Backend) error {
	err := api.Authorizer.EntityHasPermission(user, permission.SuperuserAccess, backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return api.Authorizer.EntityHasPermission(user, permission.AdminAccess, backend.ModelTag())
}

// checkControllerAdmin ensures that the logged in user is a controller admin.
func (api *BaseAPI) checkControllerAdmin() error {
	return api.Authorizer.HasPermission(permission.SuperuserAccess, api.ControllerModel.ControllerTag())
}

// modelForName looks up the model details for the named model and returns
// the model (if found), the absolute model model path which was used in the lookup,
// and a bool indicating if the model was found,
func (api *BaseAPI) modelForName(modelName, ownerName string) (Model, string, bool, error) {
	modelPath := fmt.Sprintf("%s/%s", ownerName, modelName)
	var model Model
	uuids, err := api.ControllerModel.AllModelUUIDs()
	if err != nil {
		return nil, modelPath, false, errors.Trace(err)
	}
	for _, uuid := range uuids {
		m, release, err := api.StatePool.GetModel(uuid)
		if err != nil {
			return nil, modelPath, false, errors.Trace(err)
		}
		defer release()
		if m.Name() == modelName && m.Owner().Id() == ownerName {
			model = m
			break
		}
	}
	return model, modelPath, model != nil, nil
}

func (api *BaseAPI) userDisplayName(backend Backend, userTag names.UserTag) (string, error) {
	var displayName string
	user, err := backend.User(userTag)
	if err != nil && !errors.IsNotFound(err) {
		return "", errors.Trace(err)
	} else if err == nil {
		displayName = user.DisplayName()
	}
	return displayName, nil
}

// applicationOffersFromModel gets details about remote applications that match given filters.
func (api *BaseAPI) applicationOffersFromModel(
	modelUUID string,
	user names.UserTag,
	requiredAccess permission.Access,
	filters ...jujucrossmodel.ApplicationOfferFilter,
) ([]params.ApplicationOfferAdminDetailsV5, error) {
	// Get the relevant backend for the specified model.
	backend, releaser, err := api.StatePool.Get(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer releaser()

	// If requireAdmin is true, the user must be a controller superuser
	// or model admin to proceed.
	var isAdmin bool
	err = api.checkAdmin(user, backend)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, err
	}
	isAdmin = err == nil
	if requiredAccess == permission.AdminAccess && !isAdmin {
		return nil, err
	}

	offers, err := api.GetApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiUserDisplayName, err := api.userDisplayName(backend, user)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferAdminDetailsV5
	for _, appOffer := range offers {
		userAccess := permission.AdminAccess
		// If the user is not a model admin, they need at least read
		// access on an offer to see it.
		if !isAdmin {
			if userAccess, err = api.checkOfferAccess(user, backend, appOffer.OfferUUID); err != nil {
				return nil, errors.Trace(err)
			}
			if userAccess == permission.NoAccess {
				continue
			}
			isAdmin = userAccess == permission.AdminAccess
		}
		offerParams, app, err := api.makeOfferParams(backend, &appOffer)
		// Just because we can't compose the result for one offer, log
		// that and move on to the next one.
		if err != nil {
			logger.Warningf("cannot get application offer: %v", err)
			continue
		}
		offerParams.Users = []params.OfferUserDetails{{
			UserName:    user.Id(),
			DisplayName: apiUserDisplayName,
			Access:      string(userAccess),
		}}
		offer := params.ApplicationOfferAdminDetailsV5{
			ApplicationOfferDetailsV5: *offerParams,
			ApplicationName:           appOffer.ApplicationName,
		}
		// Only admins can see some sensitive details of the offer.
		if isAdmin {
			if err := api.getOfferAdminDetails(user, backend, app, &offer); err != nil {
				logger.Warningf("cannot get offer admin details: %v", err)
			}
		}
		results = append(results, offer)
	}
	return results, nil
}

func (api *BaseAPI) getOfferAdminDetails(user names.UserTag, backend Backend, app crossmodel.Application, offer *params.ApplicationOfferAdminDetailsV5) error {
	curl, _ := app.CharmURL()
	conns, err := backend.OfferConnections(offer.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	offer.ApplicationName = app.Name()
	offer.CharmURL = *curl
	for _, oc := range conns {
		connDetails := params.OfferConnection{
			SourceModelTag: names.NewModelTag(oc.SourceModelUUID()).String(),
			Username:       oc.UserName(),
			RelationId:     oc.RelationId(),
		}
		rel, err := backend.KeyRelation(oc.RelationKey())
		if err != nil {
			return errors.Trace(err)
		}
		ep, err := rel.Endpoint(app.Name())
		if err != nil {
			return errors.Trace(err)
		}
		relStatus, err := rel.Status()
		if err != nil {
			return errors.Trace(err)
		}
		connDetails.Endpoint = ep.Name
		connDetails.Status = params.EntityStatus{
			Status: relStatus.Status,
			Info:   relStatus.Message,
			Data:   relStatus.Data,
			Since:  relStatus.Since,
		}
		relIngress, err := backend.IngressNetworks(oc.RelationKey())
		if err != nil && !errors.IsNotFound(err) {
			return errors.Trace(err)
		}
		if err == nil {
			connDetails.IngressSubnets = relIngress.CIDRS()
		}
		offer.Connections = append(offer.Connections, connDetails)
	}

	offerUsers, err := backend.GetOfferUsers(offer.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}

	for userName, access := range offerUsers {
		if userName == user.Id() {
			continue
		}
		displayName, err := api.userDisplayName(backend, names.NewUserTag(userName))
		if err != nil {
			return errors.Trace(err)
		}
		offer.Users = append(offer.Users, params.OfferUserDetails{
			UserName:    userName,
			DisplayName: displayName,
			Access:      string(access),
		})
	}
	return nil
}

// checkOfferAccess returns the level of access the authenticated user has to the offer,
// so long as it is greater than the requested perm.
func (api *BaseAPI) checkOfferAccess(user names.UserTag, backend Backend, offerUUID string) (permission.Access, error) {
	// If the authenticated user is controller superuser we return `admin`.
	err := api.Authorizer.EntityHasPermission(user, permission.SuperuserAccess, backend.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return permission.NoAccess, errors.Trace(err)
	} else if err == nil {
		return permission.AdminAccess, nil
	}

	// If the authenticated user is model administrator we return `admin`.
	err = api.Authorizer.EntityHasPermission(user, permission.AdminAccess, backend.ModelTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return permission.NoAccess, errors.Trace(err)
	} else if err == nil {
		return permission.AdminAccess, nil
	}

	// We loop through access levels in decreasing order to return the highest access level the authenticated
	// user has to the application offer.
	for _, access := range []permission.Access{permission.AdminAccess, permission.ConsumeAccess, permission.ReadAccess} {
		err := api.Authorizer.EntityHasPermission(user, access, names.NewApplicationOfferTag(offerUUID))
		if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
			return permission.NoAccess, errors.Trace(err)
		} else if err == nil {
			return access, nil
		}
	}

	return permission.NoAccess, nil
}

type offerModel struct {
	model Model
	err   error
}

// getModelsFromOffers returns a slice of models corresponding to the
// specified offer URLs. Each result item has either a model or an error.
func (api *BaseAPI) getModelsFromOffers(user names.UserTag, offerURLs ...string) ([]offerModel, error) {
	// Cache the models found so far so we don't look them up more than once.
	modelsCache := make(map[string]Model)
	oneModel := func(offerURL string) (Model, error) {
		url, err := jujucrossmodel.ParseOfferURL(offerURL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelPath := fmt.Sprintf("%s/%s", url.User, url.ModelName)
		if model, ok := modelsCache[modelPath]; ok {
			return model, nil
		}

		ownerName := url.User
		if ownerName == "" {
			ownerName = user.Id()
		}
		model, absModelPath, ok, err := api.modelForName(url.ModelName, ownerName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		if !ok {
			return nil, errors.NotFoundf("model %q", absModelPath)
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
func (api *BaseAPI) getModelFilters(user names.UserTag, filters params.OfferFilters) (
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
		ownerName := f.OwnerName
		if ownerName == "" {
			ownerName = user.Id()
		}
		var (
			modelUUID string
			ok        bool
		)
		if modelUUID, ok = modelUUIDs[f.ModelName]; !ok {
			var err error
			model, absModelPath, ok, err := api.modelForName(f.ModelName, ownerName)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			if !ok {
				err := errors.NotFoundf("model %q", absModelPath)
				return nil, nil, errors.Trace(err)
			}
			// Record the UUID and model for next time.
			modelUUID = model.UUID()
			modelUUIDs[f.ModelName] = modelUUID
			models[modelUUID] = model
		}

		// Record the filter and model details against the model UUID.
		filters := filtersPerModel[modelUUID]
		filter, err := makeOfferFilterFromParams(f)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		filters = append(filters, filter)
		filtersPerModel[modelUUID] = filters
	}
	return models, filtersPerModel, nil
}

// getApplicationOffersDetails gets details about remote applications that match given filter.
func (api *BaseAPI) getApplicationOffersDetails(
	user names.UserTag,
	filters params.OfferFilters,
	requiredPermission permission.Access,
) ([]params.ApplicationOfferAdminDetailsV5, error) {

	// If there are no filters specified, that's an error since the
	// caller is expected to specify at the least one or more models
	// to avoid an unbounded query across all models.
	if len(filters.Filters) == 0 {
		return nil, errors.New("at least one offer filter is required")
	}

	// Gather all the filter details for doing a query for each model.
	models, filtersPerModel, err := api.getModelFilters(user, filters)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Ensure the result is deterministic.
	var allUUIDs []string
	for modelUUID := range filtersPerModel {
		allUUIDs = append(allUUIDs, modelUUID)
	}
	sort.Strings(allUUIDs)

	// Do the per model queries.
	var result []params.ApplicationOfferAdminDetailsV5
	for _, modelUUID := range allUUIDs {
		filters := filtersPerModel[modelUUID]
		offers, err := api.applicationOffersFromModel(modelUUID, user, requiredPermission, filters...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		model := models[modelUUID]

		for _, offerDetails := range offers {
			offerDetails.OfferURL = jujucrossmodel.MakeURL(model.Owner().Id(), model.Name(), offerDetails.OfferName, "")
			result = append(result, offerDetails)
		}
	}
	return result, nil
}

func makeOfferFilterFromParams(filter params.OfferFilter) (jujucrossmodel.ApplicationOfferFilter, error) {
	offerFilter := jujucrossmodel.ApplicationOfferFilter{
		OfferName:              filter.OfferName,
		ApplicationName:        filter.ApplicationName,
		ApplicationDescription: filter.ApplicationDescription,
		Endpoints:              make([]jujucrossmodel.EndpointFilterTerm, len(filter.Endpoints)),
		AllowedConsumers:       make([]string, len(filter.AllowedConsumerTags)),
		ConnectedUsers:         make([]string, len(filter.ConnectedUserTags)),
	}
	for i, ep := range filter.Endpoints {
		offerFilter.Endpoints[i] = jujucrossmodel.EndpointFilterTerm{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
		}
	}
	for i, tag := range filter.AllowedConsumerTags {
		u, err := names.ParseUserTag(tag)
		if err != nil {
			return jujucrossmodel.ApplicationOfferFilter{}, errors.Trace(err)
		}
		offerFilter.AllowedConsumers[i] = u.Id()
	}
	for i, tag := range filter.ConnectedUserTags {
		u, err := names.ParseUserTag(tag)
		if err != nil {
			return jujucrossmodel.ApplicationOfferFilter{}, errors.Trace(err)
		}
		offerFilter.ConnectedUsers[i] = u.Id()
	}
	return offerFilter, nil
}

func (api *BaseAPI) makeOfferParams(backend Backend,
	offer *jujucrossmodel.ApplicationOffer,
) (*params.ApplicationOfferDetailsV5, crossmodel.Application, error) {
	app, err := backend.Application(offer.ApplicationName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	result := params.ApplicationOfferDetailsV5{
		SourceModelTag:         backend.ModelTag().String(),
		OfferName:              offer.OfferName,
		OfferUUID:              offer.OfferUUID,
		ApplicationDescription: offer.ApplicationDescription,
	}

	// Create result.Endpoints both IAAS and CAAS can use.
	for alias, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      alias,
			Interface: ep.Interface,
			Role:      ep.Role,
		})

	}

	// CAAS models don't have spaces.
	model, err := backend.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	if model.Type() == state.ModelTypeCAAS {
		return &result, app, nil
	}

	return &result, app, nil
}
