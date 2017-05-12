// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/txn"
	"gopkg.in/juju/charm.v6-unstable"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	jujucrossmodel "github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

// OffersAPI implements the cross model interface and is the concrete
// implementation of the api end point.
type OffersAPI struct {
	BaseAPI
	dataDir string
}

// createAPI returns a new application offers OffersAPI facade.
func createOffersAPI(
	getApplicationOffers func(interface{}) jujucrossmodel.ApplicationOffers,
	backend Backend,
	statePool StatePool,
	authorizer facade.Authorizer,
	resources facade.Resources,
) (*OffersAPI, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	dataDir := resources.Get("dataDir").(common.StringResource)
	api := &OffersAPI{
		dataDir: dataDir.String(),
		BaseAPI: BaseAPI{
			Authorizer:           authorizer,
			GetApplicationOffers: getApplicationOffers,
			ControllerModel:      backend,
			StatePool:            statePool,
		}}
	return api, nil
}

// NewOffersAPI returns a new application offers OffersAPI facade.
func NewOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	return createOffersAPI(
		GetApplicationOffers, GetStateAccess(ctx.State()),
		GetStatePool(ctx.StatePool()), ctx.Auth(), ctx.Resources())
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

		check := common.NewBlockChecker(backend)
		if err := check.ChangeAllowed(); err != nil {
			result[i].Error = common.ServerError(err)
			continue
		}

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
func (api *OffersAPI) ListApplicationOffers(filters params.OfferFilters) (params.ListApplicationOffersResults, error) {
	var result params.ListApplicationOffersResults
	offers, err := api.getApplicationOffersDetails(filters, true)
	if err != nil {
		return result, err
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

	url, err := jujucrossmodel.ParseApplicationURL(arg.OfferURL)
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
		access, err := backend.GetOfferAccess(offerTag, apiUser)
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
		offerAccess, err := backend.GetOfferAccess(offerTag, targetUserTag)
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
func (api *OffersAPI) ApplicationOffers(urls params.ApplicationURLs) (params.ApplicationOffersResults, error) {
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
func (api *OffersAPI) offerForURL(urlStr string) (params.ApplicationOfferDetails, error) {
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
	offers, err := api.applicationOffersFromModel(model.UUID(), false, filter)
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
func (api *OffersAPI) FindApplicationOffers(filters params.OfferFilters) (params.FindApplicationOffersResults, error) {
	var result params.FindApplicationOffersResults
	var filtersToUse params.OfferFilters

	// If there is only one filter term, and no model is specified, add in
	// any models the user can see and query across those.
	// If there's more than one filter term, each must specify a model.
	if len(filters.Filters) == 1 && filters.Filters[0].ModelName == "" {
		allModels, err := api.ControllerModel.AllModels()
		if err != nil {
			return result, errors.Trace(err)
		}
		for _, m := range allModels {
			modelFilter := filters.Filters[0]
			modelFilter.ModelName = m.Name()
			modelFilter.OwnerName = m.Owner().Name()
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

	offers, err := api.getApplicationOffersDetails(filtersToUse, false)
	if err != nil {
		return result, errors.Trace(err)
	}
	for _, offer := range offers {
		result.Results = append(result.Results, offer.ApplicationOffer)
	}
	return result, nil
}

// TODO(wallyworld) - we'll use this when the ConsumeDetails API is added.
// applicationUrlEndpointParse is used to split an application url and optional
// relation name into url and relation name.
//var applicationUrlEndpointParse = regexp.MustCompile("(?P<url>.*[/.][^:]*)(:(?P<relname>.*)$)?")

// Consume adds remote applications to the model without creating any
// relations.
func (api *OffersAPI) Consume(args params.ConsumeApplicationArgs) (params.ConsumeApplicationResults, error) {
	var consumeResults params.ConsumeApplicationResults
	results := make([]params.ConsumeApplicationResult, len(args.Args))
	for i, arg := range args.Args {
		localName, err := api.consumeOne(arg)
		results[i].LocalName = localName
		results[i].Error = common.ServerError(err)
	}
	consumeResults.Results = results
	return consumeResults, nil
}

func (api *OffersAPI) consumeOne(arg params.ConsumeApplicationArg) (string, error) {
	targetModelTag, err := names.ParseModelTag(arg.TargetModelTag)
	if err != nil {
		return "", errors.Trace(err)
	}
	sourceModelTag, err := names.ParseModelTag(arg.SourceModelTag)
	if err != nil {
		return "", errors.Trace(err)
	}

	backend, releaser, err := api.StatePool.Get(targetModelTag.Id())
	if err != nil {
		return "", errors.Trace(err)
	}
	defer releaser()

	check := common.NewBlockChecker(backend)
	if err := check.ChangeAllowed(); err != nil {
		return "", errors.Trace(err)
	}

	appName := arg.ApplicationAlias
	if appName == "" {
		appName = arg.OfferName
	}
	remoteApp, err := api.saveRemoteApplication(backend, sourceModelTag, appName, arg.OfferName, arg.OfferURL, arg.Endpoints)
	return remoteApp.Name(), err
}

// saveRemoteApplication saves the details of the specified remote application and its endpoints
// to the state model so relations to the remote application can be created.
func (api *OffersAPI) saveRemoteApplication(
	backend Backend,
	sourceModelTag names.ModelTag, applicationName, offerName, url string, endpoints []params.RemoteEndpoint,
) (RemoteApplication, error) {
	remoteEps := make([]charm.Relation, len(endpoints))
	for j, ep := range endpoints {
		remoteEps[j] = charm.Relation{
			Name:      ep.Name,
			Role:      ep.Role,
			Interface: ep.Interface,
			Limit:     ep.Limit,
			Scope:     ep.Scope,
		}
	}

	// If the a remote application with the same name and endpoints from the same
	// source model already exists, we will use that one.
	remoteApp, err := api.maybeUpdateExistingApplicationEndpoints(backend, applicationName, sourceModelTag, remoteEps)
	if err == nil {
		return remoteApp, nil
	} else if !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}

	return backend.AddRemoteApplication(state.AddRemoteApplicationParams{
		Name:        applicationName,
		OfferName:   offerName,
		URL:         url,
		SourceModel: sourceModelTag,
		Endpoints:   remoteEps,
	})
}

// maybeUpdateExistingApplicationEndpoints looks for a remote application with the
// specified name and source model tag and tries to update its endpoints with the
// new ones specified. If the endpoints are compatible, the newly updated remote
// application is returned.
func (api *OffersAPI) maybeUpdateExistingApplicationEndpoints(
	backend Backend, applicationName string, sourceModelTag names.ModelTag, remoteEps []charm.Relation,
) (RemoteApplication, error) {
	existingRemoteApp, err := backend.RemoteApplication(applicationName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err != nil && !errors.IsNotFound(err) {
		return nil, errors.Trace(err)
	}
	if existingRemoteApp.SourceModel().Id() != sourceModelTag.Id() {
		return nil, errors.AlreadyExistsf("remote application called %q from a different model", applicationName)
	}
	newEpsMap := make(map[charm.Relation]bool)
	for _, ep := range remoteEps {
		newEpsMap[ep] = true
	}
	existingEps, err := existingRemoteApp.Endpoints()
	if err != nil {
		return nil, errors.Trace(err)
	}
	maybeSameEndpoints := len(newEpsMap) == len(existingEps)
	existingEpsByName := make(map[string]charm.Relation)
	for _, ep := range existingEps {
		existingEpsByName[ep.Name] = ep.Relation
		delete(newEpsMap, ep.Relation)
	}
	sameEndpoints := maybeSameEndpoints && len(newEpsMap) == 0
	if sameEndpoints {
		return existingRemoteApp, nil
	}

	// Gather the new endpoints. All new endpoints passed to AddEndpoints()
	// below must not have the same name as an existing endpoint.
	var newEps []charm.Relation
	for ep := range newEpsMap {
		// See if we are attempting to update endpoints with the same name but
		// different relation data.
		if existing, ok := existingEpsByName[ep.Name]; ok && existing != ep {
			return nil, errors.Errorf("conflicting endpoint %v", ep.Name)
		}
		newEps = append(newEps, ep)
	}

	if len(newEps) > 0 {
		// Update the existing remote app to have the new, additional endpoints.
		if err := existingRemoteApp.AddEndpoints(newEps); err != nil {
			return nil, errors.Trace(err)
		}
	}
	return existingRemoteApp, nil
}

// RemoteApplicationInfo returns information about the requested remote application.
func (api *OffersAPI) RemoteApplicationInfo(args params.ApplicationURLs) (params.RemoteApplicationInfoResults, error) {
	results := make([]params.RemoteApplicationInfoResult, len(args.ApplicationURLs))
	for i, url := range args.ApplicationURLs {
		info, err := api.oneRemoteApplicationInfo(url)
		results[i].Result = info
		results[i].Error = common.ServerError(err)
	}
	return params.RemoteApplicationInfoResults{results}, nil
}

func (api *OffersAPI) oneRemoteApplicationInfo(urlStr string) (*params.RemoteApplicationInfo, error) {
	url, err := jujucrossmodel.ParseApplicationURL(urlStr)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// We need at least read access to the model to see the application details.
	offer, sourceModelTag, err := api.offeredApplicationDetails(url, permission.ReadAccess)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &params.RemoteApplicationInfo{
		ModelTag:         sourceModelTag.String(),
		Name:             url.ApplicationName,
		Description:      offer.ApplicationDescription,
		ApplicationURL:   url.String(),
		SourceModelLabel: url.ModelName,
		Endpoints:        offer.Endpoints,
		IconURLPath:      fmt.Sprintf("rest/1.0/remote-application/%s/icon", url.ApplicationName),
	}, nil
}

// offeredApplicationDetails returns details of the application offered at the specified URL.
// The user is required to have the specified permission on the offer.
func (api *OffersAPI) offeredApplicationDetails(url *jujucrossmodel.ApplicationURL, perm permission.Access) (
	offer *params.ApplicationOffer,
	sourceModelTag names.ModelTag,
	err error,
) {
	fail := func(err error) (
		*params.ApplicationOffer,
		names.ModelTag,
		error,
	) {
		fmt.Println("FAIL: " + err.Error())
		return nil, sourceModelTag, err
	}

	// We require the hosting model to be specified.
	if url.ModelName == "" {
		return fail(errors.Errorf("missing model name in URL %q", url.String()))
	}

	models, err := api.getModelsFromOffers(url.String())
	if err != nil {
		return fail(errors.Trace(err))
	}
	one := models[0]
	if one.err != nil {
		return fail(errors.Trace(one.err))
	}
	sourceModelTag = one.model.ModelTag()

	app, releaser, err := api.offeredApplication(sourceModelTag, url.ApplicationName, perm)
	if err != nil {
		return fail(errors.Trace(err))
	}
	defer releaser()
	offer, err = api.makeOfferParamsFromApplication(sourceModelTag, url.ApplicationName, app)
	return offer, sourceModelTag, err
}

func (api *OffersAPI) offeredApplication(sourceModelTag names.ModelTag, offerName string, perm permission.Access) (
	_ Application,
	releaser func(),
	err error,
) {
	defer func() {
		if err != nil && releaser != nil {
			releaser()
		}
	}()

	fail := func(err error) (
		Application,
		func(),
		error,
	) {
		return nil, releaser, err
	}

	// Get the backend state for the source model so we can lookup the application.
	var backend Backend
	backend, releaser, err = api.StatePool.Get(sourceModelTag.Id())
	if err != nil {
		return fail(errors.Trace(err))
	}

	// For now, offer URL is matched against the specified application
	// name as seen from the consuming model.
	offers, err := api.GetApplicationOffers(backend).ListOffers(
		jujucrossmodel.ApplicationOfferFilter{
			OfferName: offerName,
		},
	)
	if err != nil {
		return fail(errors.Trace(err))
	}

	// The offers query succeeded but there were no offers matching the required offer name.
	if len(offers) == 0 {
		return fail(errors.NotFoundf("application offer %q", offerName))
	}
	// Sanity check - this should never happen.
	if len(offers) > 1 {
		return fail(errors.Errorf("unexpected: %d matching offers for %q", len(offers), offerName))
	}

	// Check the permissions - a user can access the offer if they are an admin
	// or they have consume access to the offer.
	isAdmin := false
	err = api.checkPermission(backend.ControllerTag(), permission.SuperuserAccess)
	if err == common.ErrPerm {
		err = api.checkPermission(sourceModelTag, permission.AdminAccess)
	}
	if err != nil && err != common.ErrPerm {
		return fail(errors.Trace(err))
	}
	isAdmin = err == nil

	offer := offers[0]
	if !isAdmin {
		// Check for consume access on tne offer - we can't use api.checkPermission as
		// we need to operate on the state containing the offer.
		access, err := api.checkOfferAccess(backend, offerName, perm)
		if err != nil {
			return fail(errors.Trace(err))
		}
		if access == permission.NoAccess {
			return fail(common.ErrPerm)
		}
	}
	app, err := backend.Application(offer.ApplicationName)
	if err != nil {
		return fail(errors.Trace(err))
	}
	return app, releaser, err
}

func (api *OffersAPI) makeOfferParamsFromApplication(
	sourceModelTag names.ModelTag,
	offerName string,
	app Application,
) (*params.ApplicationOffer, error) {
	ch, _, err := app.Charm()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := params.ApplicationOffer{
		SourceModelTag:         sourceModelTag.String(),
		OfferName:              offerName,
		ApplicationDescription: ch.Meta().Description,
	}
	eps, err := app.Endpoints()
	for _, ep := range eps {
		result.Endpoints = append(result.Endpoints, params.RemoteEndpoint{
			Name:      ep.Name,
			Interface: ep.Interface,
			Role:      ep.Role,
			Scope:     ep.Scope,
			Limit:     ep.Limit,
		})
	}
	return &result, nil
}
