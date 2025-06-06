// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"fmt"
	"sort"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	coreerrors "github.com/juju/juju/core/errors"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	coreuser "github.com/juju/juju/core/user"
	accesserrors "github.com/juju/juju/domain/access/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	interrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// BaseAPI provides various boilerplate methods used by the facade business logic.
type BaseAPI struct {
	Authorizer                facade.Authorizer
	GetApplicationOffers      func(interface{}) jujucrossmodel.ApplicationOffers
	ControllerModel           Backend
	StatePool                 StatePool
	accessService             AccessService
	modelDomainServicesGetter ModelDomainServicesGetter
	getControllerInfo         func(context.Context) (apiAddrs []string, caCert string, _ error)
	logger                    corelogger.Logger
	controllerUUID            string
	modelService              ModelService
}

// checkAdmin ensures that the specified in user is a model or controller admin.
func (api *BaseAPI) checkAdmin(
	ctx context.Context, user names.UserTag, modelUUID model.UUID,
) error {
	controllerTag := names.NewControllerTag(api.controllerUUID)
	err := api.Authorizer.EntityHasPermission(ctx, user, permission.SuperuserAccess, controllerTag)
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	} else if err == nil {
		return nil
	}

	return api.Authorizer.EntityHasPermission(ctx, user, permission.AdminAccess, names.NewModelTag(modelUUID.String()))
}

// checkControllerAdmin ensures that the logged in user is a controller admin.
func (api *BaseAPI) checkControllerAdmin(ctx context.Context) error {
	controllerTag := names.NewControllerTag(api.controllerUUID)
	return api.Authorizer.HasPermission(ctx, permission.SuperuserAccess, controllerTag)
}

// modelForName returns the model details for the specified model name,
// along with the absolute model path used in the lookup.
//
// The following errors may be returned:
// - [coreerrors.NotFound] when no model with the given name exists.
// - [coreerrors.NotValid] when ownerName is not valid.
func (api *BaseAPI) modelForName(ctx context.Context, modelName, ownerName string) (model.Model, error) {
	modelPath := fmt.Sprintf("%s/%s", ownerName, modelName)
	// TODO - GetModelByNameAndOwner is being renamed to GetModelByNameAndQualifier
	qualifier := model.QualifierFromUserTag(names.NewUserTag(ownerName))
	qualifierName, err := coreuser.NewName(qualifier.String())
	if err != nil {
		return model.Model{}, errors.Trace(err)
	}
	m, err := api.modelService.GetModelByNameAndOwner(ctx, modelName, qualifierName)
	if interrors.Is(err, modelerrors.NotFound) {
		return model.Model{}, interrors.Errorf("model %q %w", modelPath, coreerrors.NotFound)
	} else if interrors.Is(err, accesserrors.UserNameNotValid) {
		return model.Model{}, interrors.Errorf("user name %q %w", ownerName, coreerrors.NotValid)
	}

	return m, errors.Trace(err)
}

func (api *BaseAPI) userDisplayName(ctx context.Context, userName coreuser.Name) (string, error) {
	var displayName string
	user, err := api.accessService.GetUserByName(ctx, userName)
	if err != nil && !errors.Is(err, accesserrors.UserNotFound) {
		return "", errors.Trace(err)
	} else if err == nil {
		displayName = user.DisplayName
	}
	return displayName, nil
}

// applicationOffersFromModel gets details about remote applications that match given filters.
func (api *BaseAPI) applicationOffersFromModel(
	ctx context.Context,
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
	err = api.checkAdmin(ctx, user, model.UUID(modelUUID))
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return nil, err
	}
	isAdmin = err == nil
	if requiredAccess == permission.AdminAccess && !isAdmin {
		return nil, err
	}

	// TODO(aflynn): re-enable filtering by allowed consumers in domain and
	// remove this warning.
	for _, filter := range filters {
		if len(filter.AllowedConsumers) > 0 {
			api.logger.Warningf(ctx, "filtering by allowed consumer is disabled due to the migration of offer permissions to domain")
			break
		}
	}

	offers, err := api.GetApplicationOffers(backend).ListOffers(filters...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	apiUserDisplayName, err := api.userDisplayName(ctx, coreuser.NameFromTag(user))
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []params.ApplicationOfferAdminDetailsV5
	for _, appOffer := range offers {
		userAccess := permission.AdminAccess
		// If the user is not a model admin, they need at least read
		// access on an offer to see it.
		if !isAdmin {
			if userAccess, err = api.checkOfferAccess(ctx, coreuser.NameFromTag(user), appOffer.OfferUUID); err != nil {
				return nil, errors.Trace(err)
			}
			if userAccess == permission.NoAccess {
				continue
			}
			isAdmin = userAccess == permission.AdminAccess
		}
		offerParams, app, err := api.makeOfferParams(model.UUID(modelUUID), backend, &appOffer)
		// Just because we can't compose the result for one offer, log
		// that and move on to the next one.
		if err != nil {
			api.logger.Warningf(ctx, "cannot get application offer: %v", err)
			continue
		}
		offerParams.Users = []params.OfferUserDetails{{
			UserName:    user.Id(),
			DisplayName: apiUserDisplayName,
			Access:      string(userAccess),
		}}
		offer := params.ApplicationOfferAdminDetailsV5{
			ApplicationOfferDetailsV5: *offerParams,
		}
		// Only admins can see some sensitive details of the offer.
		if isAdmin {
			if err := api.getOfferAdminDetails(ctx, user, backend, app, &offer); err != nil {
				api.logger.Warningf(ctx, "cannot get offer admin details: %v", err)
			}
		}
		results = append(results, offer)
	}
	return results, nil
}

func (api *BaseAPI) getOfferAdminDetails(
	ctx context.Context,
	user names.UserTag,
	backend Backend,
	app crossmodel.Application,
	offer *params.ApplicationOfferAdminDetailsV5,
) error {
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
		if err != nil && !errors.Is(err, errors.NotFound) {
			return errors.Trace(err)
		}
		if err == nil {
			connDetails.IngressSubnets = relIngress.CIDRS()
		}
		offer.Connections = append(offer.Connections, connDetails)
	}

	userAccesses, err := api.accessService.ReadAllUserAccessForTarget(ctx, permission.ID{
		ObjectType: permission.Offer,
		Key:        offer.OfferUUID,
	})
	if err != nil {
		return errors.Trace(err)
	}

	for _, userAccess := range userAccesses {
		if userAccess.UserName.Name() == user.Id() {
			continue
		}
		offer.Users = append(offer.Users, params.OfferUserDetails{
			UserName:    userAccess.UserName.Name(),
			DisplayName: userAccess.DisplayName,
			Access:      string(userAccess.Access),
		})
	}
	return nil
}

// checkOfferAccess returns the level of access the authenticated user has to the offer,
// so long as it is greater than the requested perm.
func (api *BaseAPI) checkOfferAccess(ctx context.Context, user coreuser.Name, offerUUID string) (permission.Access, error) {
	access, err := api.accessService.ReadUserAccessLevelForTarget(ctx, user, permission.ID{
		ObjectType: permission.Offer,
		Key:        offerUUID,
	})
	if err != nil && !errors.Is(err, accesserrors.AccessNotFound) {
		return permission.NoAccess, errors.Trace(err)
	}
	if !access.EqualOrGreaterOfferAccessThan(permission.ReadAccess) {
		return permission.NoAccess, nil
	}
	return access, nil
}

type offerModel struct {
	model model.Model
	err   error
}

// getModelsFromOffers returns a slice of models corresponding to the
// specified offer URLs. Each result item has either a model or an error.
func (api *BaseAPI) getModelsFromOffers(ctx context.Context, user names.UserTag, offerURLs ...string) ([]offerModel, error) {
	// Cache the models found so far so we don't look them up more than once.
	modelsCache := make(map[string]model.Model)
	oneModel := func(offerURL string) (model.Model, error) {
		url, err := jujucrossmodel.ParseOfferURL(offerURL)
		if err != nil {
			return model.Model{}, errors.Trace(err)
		}
		modelPath := fmt.Sprintf("%s/%s", url.User, url.ModelName)
		if model, ok := modelsCache[modelPath]; ok {
			return model, nil
		}

		ownerName := url.User
		if ownerName == "" {
			ownerName = user.Id()
		}
		m, err := api.modelForName(ctx, url.ModelName, ownerName)
		if err != nil {
			return model.Model{}, errors.Trace(err)
		}
		return m, nil
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
func (api *BaseAPI) getModelFilters(ctx context.Context, user names.UserTag, filters params.OfferFilters) (
	models map[string]model.Model,
	filtersPerModel map[string][]jujucrossmodel.ApplicationOfferFilter,
	_ error,
) {
	models = make(map[string]model.Model)
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
			model, err := api.modelForName(ctx, f.ModelName, ownerName)
			if err != nil {
				return nil, nil, errors.Trace(err)
			}
			// Record the UUID and model for next time.
			modelUUID = model.UUID.String()
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
	ctx context.Context,
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
	models, filtersPerModel, err := api.getModelFilters(ctx, user, filters)
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
		offers, err := api.applicationOffersFromModel(ctx, modelUUID, user, requiredPermission, filters...)
		if err != nil {
			return nil, errors.Trace(err)
		}
		m := models[modelUUID]
		ownerTag, err := model.UserTagFromQualifier(m.Qualifier)
		if err != nil {
			return nil, errors.Trace(err)
		}

		for _, offerDetails := range offers {
			offerDetails.OfferURL = jujucrossmodel.MakeURL(ownerTag.Id(), m.Name, offerDetails.OfferName, "")
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

func (api *BaseAPI) makeOfferParams(
	modelUUID model.UUID,
	backend Backend,
	offer *jujucrossmodel.ApplicationOffer,
) (*params.ApplicationOfferDetailsV5, crossmodel.Application, error) {
	app, err := backend.Application(offer.ApplicationName)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	result := params.ApplicationOfferDetailsV5{
		SourceModelTag:         names.NewModelTag(modelUUID.String()).String(),
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

	return &result, app, nil
}
