// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/utils/set"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/permission"
)

// BaseAPI provides various boilerplate methods used by the facade business logic.
type BaseAPI struct {
	Authorizer           facade.Authorizer
	GetApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers
	ControllerModel      Backend
	StatePool            StatePool
	getEnviron           environFromModelFunc
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
	user := api.Authorizer.GetAuthTag()
	if ownerName == "" {
		ownerName = user.Id()
	}
	var model Model
	uuids, err := api.ControllerModel.AllModelUUIDs()
	if err != nil {
		return nil, false, errors.Trace(err)
	}
	for _, uuid := range uuids {
		m, release, err := api.StatePool.GetModel(uuid)
		if err != nil {
			return nil, false, errors.Trace(err)
		}
		defer release()
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
	requiredAccess permission.Access,
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
	if requiredAccess == permission.AdminAccess && !isAdmin {
		return nil, common.ErrPerm
	}

	offers, err := api.GetApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferDetails
	for _, appOffer := range offers {
		userAccess := permission.AdminAccess
		// If the user is not a model admin, they need at least read
		// access on an offer to see it.
		if !isAdmin {
			if userAccess, err = api.checkOfferAccess(backend, appOffer.OfferUUID, requiredAccess); err != nil {
				return nil, errors.Trace(err)
			}
			if userAccess == permission.NoAccess {
				continue
			}
			isAdmin = userAccess == permission.AdminAccess
		}
		offerParams, app, err := api.makeOfferParams(backend, &appOffer, userAccess)
		// Just because we can't compose the result for one offer, log
		// that and move on to the next one.
		if err != nil {
			logger.Warningf("cannot get application offer: %v", err)
			continue
		}
		offer := params.ApplicationOfferDetails{
			ApplicationOffer: *offerParams,
		}
		// Only admins can see some sensitive details of the offer.
		if isAdmin {
			curl, _ := app.CharmURL()
			conns, err := backend.OfferConnections(offer.OfferUUID)
			if err != nil {
				logger.Warningf("cannot get offer connection details: %v", err)
				continue
			}
			offer.ApplicationName = app.Name()
			offer.CharmURL = curl.String()
			for _, oc := range conns {
				connDetails := params.OfferConnection{
					SourceModelTag: names.NewModelTag(oc.SourceModelUUID()).String(),
					Username:       oc.UserName(),
					RelationId:     oc.RelationId(),
					// TODO(wallyworld)
					Status: "active",
				}
				rel, err := backend.KeyRelation(oc.RelationKey())
				if err != nil {
					return nil, errors.Trace(err)
				}
				ep, err := rel.Endpoint(app.Name())
				if err != nil {
					return nil, errors.Trace(err)
				}
				connDetails.Endpoint = ep.Name
				offer.Connections = append(offer.Connections, connDetails)
			}
		}
		results = append(results, offer)
	}
	return results, nil
}

// checkOfferAccess returns the level of access the authenticated user has to the offer,
// so long as it is greater than the requested perm.
func (api *BaseAPI) checkOfferAccess(backend Backend, offerUUID string, perm permission.Access) (permission.Access, error) {
	apiUser := api.Authorizer.GetAuthTag().(names.UserTag)
	access, err := backend.GetOfferAccess(offerUUID, apiUser)
	if err != nil && !errors.IsNotFound(err) {
		return permission.NoAccess, errors.Trace(err)
	}
	if !access.EqualOrGreaterOfferAccessThan(permission.ReadAccess) {
		return permission.NoAccess, nil
	}
	return access, nil
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
	requiredPermission permission.Access,
) ([]params.ApplicationOfferDetails, error) {

	// If there are no filters specified, that's an error since the
	// caller is expected to specify at the least one or more models
	// to avoid an unbounded query across all models.
	if len(filters.Filters) == 0 {
		return nil, errors.New("at least one offer filter is required")
	}

	// Gather all the filter details for doing a query for each model.
	models, filtersPerModel, err := api.getModelFilters(filters)
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
	var result []params.ApplicationOfferDetails
	for _, modelUUID := range allUUIDs {
		filters := filtersPerModel[modelUUID]
		offers, err := api.applicationOffersFromModel(modelUUID, requiredPermission, filters...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		model := models[modelUUID]

		for _, offerDetails := range offers {
			offerDetails.OfferURL = jujucrossmodel.MakeURL(model.Owner().Name(), model.Name(), offerDetails.OfferName, "")
			result = append(result, offerDetails)
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

func (api *BaseAPI) makeOfferParams(backend Backend, offer *jujucrossmodel.ApplicationOffer, access permission.Access) (
	*params.ApplicationOffer, crossmodel.Application, error,
) {
	app, err := backend.Application(offer.ApplicationName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	appBindings, err := app.EndpointBindings()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	result := params.ApplicationOffer{
		SourceModelTag:         backend.ModelTag().String(),
		OfferName:              offer.OfferName,
		OfferUUID:              offer.OfferUUID,
		ApplicationDescription: offer.ApplicationDescription,
		Access:                 string(access),
	}

	spaceNames := set.NewStrings()
	for alias, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      alias,
			Interface: ep.Interface,
			Role:      ep.Role,
		})
		spaceName, ok := appBindings[ep.Name]
		if !ok {
			// There should always be some binding (even if it's to the default space).
			// This isn't currently the case so add the default binding here.
			logger.Warningf("no binding for %q endpoint on application %q", ep.Name, offer.ApplicationName)
			if result.Bindings == nil {
				result.Bindings = make(map[string]string)
			}
			result.Bindings[ep.Name] = environs.DefaultSpaceName
		}
		spaceNames.Add(spaceName)
	}

	spaces, err := api.collectRemoteSpaces(backend, spaceNames.SortedValues())
	if errors.IsNotSupported(err) {
		// Provider doesn't support ProviderSpaceInfo; continue
		// without any space information, we shouldn't short-circuit
		// cross-model connections.
		return &result, app, nil
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	// Ensure bindings only contains entries for which we have spaces.
	for epName, spaceName := range appBindings {
		space, ok := spaces[spaceName]
		if !ok {
			continue
		}
		if result.Bindings == nil {
			result.Bindings = make(map[string]string)
		}
		result.Bindings[epName] = spaceName
		result.Spaces = append(result.Spaces, space)
	}
	return &result, app, nil
}

// collectRemoteSpaces gets provider information about the spaces from
// the state passed in. (This state will be for a different model than
// this API instance, which is why the results are *remote* spaces.)
// These can be used by the provider later on to decide whether a
// connection can be made via cloud-local addresses. If the provider
// doesn't support getting ProviderSpaceInfo the NotSupported error
// will be returned.
func (api *BaseAPI) collectRemoteSpaces(backend Backend, spaceNames []string) (map[string]params.RemoteSpace, error) {
	env, err := api.getEnviron(backend.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	netEnv, ok := environs.SupportsNetworking(env)
	if !ok {
		logger.Debugf("cloud provider doesn't support networking, not getting space info")
		return nil, nil
	}

	results := make(map[string]params.RemoteSpace)
	for _, name := range spaceNames {
		space := environs.DefaultSpaceInfo
		if name != environs.DefaultSpaceName {
			dbSpace, err := backend.Space(name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			space, err = spaceInfoFromState(dbSpace)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		providerSpace, err := netEnv.ProviderSpaceInfo(space)
		if err != nil && !errors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		if providerSpace == nil {
			logger.Warningf("no provider space info for %q", name)
			continue
		}
		remoteSpace := paramsFromProviderSpaceInfo(providerSpace)
		// Use the name from state in case provider and state disagree.
		remoteSpace.Name = name
		results[name] = remoteSpace
	}
	return results, nil
}
