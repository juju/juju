// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"fmt"
	"sort"

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/permission"
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
	callContext          context.ProviderCallContext
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

// modelForName looks up the model details for the named model and returns
// the model (if found), the absolute model model path which was used in the lookup,
// and a bool indicating if the model was found,
func (api *BaseAPI) modelForName(modelName, ownerName string) (Model, string, bool, error) {
	user := api.Authorizer.GetAuthTag()
	if ownerName == "" {
		ownerName = user.Id()
	}
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
	requiredAccess permission.Access,
	filters ...jujucrossmodel.ApplicationOfferFilter,
) ([]params.ApplicationOfferAdminDetails, error) {
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

	apiUserTag := api.Authorizer.GetAuthTag().(names.UserTag)
	apiUserDisplayName, err := api.userDisplayName(backend, apiUserTag)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferAdminDetails
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
		offerParams, app, err := api.makeOfferParams(backend, &appOffer)
		// Just because we can't compose the result for one offer, log
		// that and move on to the next one.
		if err != nil {
			logger.Warningf("cannot get application offer: %v", err)
			continue
		}
		offerParams.Users = []params.OfferUserDetails{{
			UserName:    apiUserTag.Id(),
			DisplayName: apiUserDisplayName,
			Access:      string(userAccess),
		}}
		offer := params.ApplicationOfferAdminDetails{
			ApplicationOfferDetails: *offerParams,
		}
		// Only admins can see some sensitive details of the offer.
		if isAdmin {
			if err := api.getOfferAdminDetails(backend, app, &offer); err != nil {
				logger.Warningf("cannot get offer admin details: %v", err)
			}
		}
		results = append(results, offer)
	}
	return results, nil
}

func (api *BaseAPI) getOfferAdminDetails(backend Backend, app crossmodel.Application, offer *params.ApplicationOfferAdminDetails) error {
	curl, _ := app.CharmURL()
	conns, err := backend.OfferConnections(offer.OfferUUID)
	if err != nil {
		return errors.Trace(err)
	}
	offer.ApplicationName = app.Name()
	offer.CharmURL = curl.String()
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

	apiUserTag := api.Authorizer.GetAuthTag().(names.UserTag)
	for userName, access := range offerUsers {
		if userName == apiUserTag.Id() {
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
		url, err := charm.ParseOfferURL(offerURL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		modelPath := fmt.Sprintf("%s/%s", url.User, url.ModelName)
		if model, ok := modelsCache[modelPath]; ok {
			return model, nil
		}

		model, absModelPath, ok, err := api.modelForName(url.ModelName, url.User)
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
			model, absModelPath, ok, err := api.modelForName(f.ModelName, f.OwnerName)
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
	filters params.OfferFilters,
	requiredPermission permission.Access,
) ([]params.ApplicationOfferAdminDetails, error) {

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
	var result []params.ApplicationOfferAdminDetails
	for _, modelUUID := range allUUIDs {
		filters := filtersPerModel[modelUUID]
		offers, err := api.applicationOffersFromModel(modelUUID, requiredPermission, filters...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		model := models[modelUUID]

		for _, offerDetails := range offers {
			offerDetails.OfferURL = charm.MakeURL(model.Owner().Name(), model.Name(), offerDetails.OfferName, "")
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

func (api *BaseAPI) makeOfferParams(backend Backend, offer *jujucrossmodel.ApplicationOffer) (
	*params.ApplicationOfferDetails, crossmodel.Application, error,
) {
	app, err := backend.Application(offer.ApplicationName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	appBindings, err := app.EndpointBindings()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	result := params.ApplicationOfferDetails{
		SourceModelTag:         backend.ModelTag().String(),
		OfferName:              offer.OfferName,
		OfferUUID:              offer.OfferUUID,
		ApplicationDescription: offer.ApplicationDescription,
	}

	// CAAS models don't have spaces.
	model, err := backend.Model()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	spaceNames := set.NewStrings()
	for alias, ep := range offer.Endpoints {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      alias,
			Interface: ep.Interface,
			Role:      ep.Role,
		})
		if model.Type() == state.ModelTypeCAAS {
			continue
		}

		spaceName, ok := appBindings[ep.Name]
		if !ok {
			// There should always be some binding (even if it's to the default space).
			// This isn't currently the case so add the default binding here.
			logger.Warningf("no binding for %q endpoint on application %q", ep.Name, offer.ApplicationName)
			if result.Bindings == nil {
				result.Bindings = make(map[string]string)
			}
			result.Bindings[ep.Name] = network.DefaultSpaceName
		}
		spaceNames.Add(spaceName)
	}

	if model.Type() == state.ModelTypeCAAS {
		return &result, app, nil
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
		if name != network.DefaultSpaceName {
			dbSpace, err := backend.Space(name)
			if err != nil {
				return nil, errors.Trace(err)
			}
			space, err = spaceInfoFromState(dbSpace)
			if err != nil {
				return nil, errors.Trace(err)
			}
		}
		providerSpace, err := netEnv.ProviderSpaceInfo(api.callContext, space)
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
