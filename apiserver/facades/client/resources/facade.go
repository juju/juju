// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"
	"fmt"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	apiresources "github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms"
	apiservercharms "github.com/juju/juju/apiserver/internal/charms"
	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	coreresource "github.com/juju/juju/core/resource"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/resource"
	resourceerrors "github.com/juju/juju/domain/resource/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	charmresource "github.com/juju/juju/internal/charm/resource"
	internalerrors "github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// API is the public API facade for resources.
type API struct {
	applicationService ApplicationService
	resourceService    ResourceService

	factory func(context.Context, *charm.URL) (NewCharmRepository, error)
	logger  corelogger.Logger
}

// NewFacade creates a public API facade for resources. It is
// used for API registration.
func NewFacade(ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	modelConfigService := ctx.DomainServices().Config()

	charmhubHTTPClient, err := ctx.HTTPClient(corehttp.CharmhubPurpose)
	if err != nil {
		return nil, fmt.Errorf(
			"getting charm hub http client: %w",
			err,
		)
	}

	logger := ctx.Logger().Child("resources")
	factory := func(stdCtx context.Context, curl *charm.URL) (NewCharmRepository, error) {
		schema := curl.Schema
		switch {
		case charm.CharmHub.Matches(schema):
			httpClient := charmhubHTTPClient
			modelCfg, err := modelConfigService.ModelConfig(stdCtx)
			if err != nil {
				return nil, fmt.Errorf("getting model config %w", err)
			}
			chURL, _ := modelCfg.CharmHubURL()

			return repository.NewCharmHubRepository(repository.CharmHubRepositoryConfig{
				CharmhubHTTPClient: httpClient,
				CharmhubURL:        chURL,
				Logger:             logger,
			})

		case charm.Local.Matches(schema):
			return &localClient{}, nil

		default:
			return nil, errors.Errorf("unrecognized charm schema %q", curl.Schema)
		}
	}

	f, err := NewResourcesAPI(ctx.DomainServices().Application(), ctx.DomainServices().Resource(), factory, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return f, nil
}

// NewResourcesAPI returns a new resources API facade.
func NewResourcesAPI(
	applicationService ApplicationService,
	resourceService ResourceService,
	factory func(context.Context, *charm.URL) (NewCharmRepository, error),
	logger corelogger.Logger,
) (*API, error) {
	if applicationService == nil {
		return nil, errors.Errorf("missing application service")
	}
	if resourceService == nil {
		return nil, errors.Errorf("missing resource service")
	}
	if factory == nil {
		// Technically this only matters for one code path through
		// AddPendingResources(). However, that functionality should be
		// provided. So we indicate the problem here instead of later
		// in the specific place where it actually matters.
		return nil, errors.Errorf("missing factory for new repository")
	}

	f := &API{
		applicationService: applicationService,
		resourceService:    resourceService,
		factory:            factory,
		logger:             logger,
	}
	return f, nil
}

// ListResources returns the list of resources for the given application.
func (a *API) ListResources(ctx context.Context, args params.ListResourcesArgs) (params.ResourcesResults, error) {
	var r params.ResourcesResults
	r.Results = make([]params.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		a.logger.Tracef(ctx, "Listing resources for %q", e.Tag)
		tag, apierr := parseApplicationTag(e.Tag)
		if apierr != nil {
			r.Results[i] = params.ResourcesResult{
				ErrorResult: params.ErrorResult{
					Error: apierr,
				},
			}
			continue
		}

		appID, err := a.applicationService.GetApplicationIDByName(ctx, tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		svcRes, err := a.resourceService.ListResources(ctx, appID)
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		r.Results[i] = applicationResources2APIResult(svcRes)
	}
	return r, nil
}

// AddPendingResources handles 2 scenarios
//  1. Adds the provided resources (info) to the Juju model before the
//     application exists. These resources are resolved when the
//     application is created using the returned Resource UUIDs.
//  2. Updates which resource revision an application uses, changing the
//     origin to store. No Resource IDs are returned.
//
// Handles CharmHub and Local charms.
func (a *API) AddPendingResources(
	ctx context.Context,
	args params.AddPendingResourcesArgsV2,
) (params.AddPendingResourcesResult, error) {
	var result params.AddPendingResourcesResult

	tag, apiErr := parseApplicationTag(args.Tag)
	if apiErr != nil {
		result.Error = apiErr
		return result, nil
	}
	appName := tag.Id()

	requestedOrigin, err := charms.ConvertParamsOrigin(args.CharmOrigin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	charmLocator, curl, err := a.getCharmLocatorAndURL(args.URL)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	var resources []charmresource.Resource
	for _, apiRes := range args.Resources {
		res, err := apiresources.API2CharmResource(apiRes)
		if err != nil {
			result.Error = apiservererrors.ServerError(errors.Annotatef(err, "bad resource info for %q", apiRes.Name))
			return result, nil
		}

		resources = append(resources, res)
	}

	resolvedResources, err := a.resolveResources(ctx, curl, requestedOrigin, resources)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}

	applicationID, err := a.applicationService.GetApplicationIDByName(ctx, appName)
	if err == nil {
		// The application does exist, therefore the intent is to
		// update a resource.
		newUUIDs, err := a.updateResources(ctx, applicationID, resolvedResources)
		result.Error = apiservererrors.ServerError(err)
		result.PendingIDs = newUUIDs
		return result, nil
	} else if !errors.Is(err, applicationerrors.ApplicationNotFound) {
		return result, internalerrors.Capture(err)
	}

	ids, err := a.addPendingResources(ctx, appName, charmLocator, resolvedResources)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.PendingIDs = ids
	return result, nil
}

func (a *API) resolveResources(
	ctx context.Context,
	curl *charm.URL,
	origin corecharm.Origin,
	resources []charmresource.Resource,
) ([]charmresource.Resource, error) {
	id := corecharm.CharmID{
		URL:    curl,
		Origin: origin,
	}
	repo, err := a.factory(ctx, id.URL)
	if err != nil {
		return nil, errors.Trace(err)
	}

	resolvedResources, err := repo.ResolveResources(ctx, resources, id)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return resolvedResources, nil
}

func (a *API) getCharmLocatorAndURL(
	charmURLStr string,
) (applicationcharm.CharmLocator, *charm.URL, error) {
	curl, err := charm.ParseURL(charmURLStr)
	if err != nil {
		return applicationcharm.CharmLocator{}, nil,
			errors.Annotatef(err, "parsing charm URL %q", charmURLStr)
	}
	charmLocator, err := apiservercharms.CharmLocatorFromURL(charmURLStr)
	return charmLocator, curl, err
}

// addPendingResources transforms and validates the resource
// data before saving the intention of which resource blob
// to use with an application currently being deployed.
func (a *API) addPendingResources(
	ctx context.Context,
	appName string,
	charmLocator applicationcharm.CharmLocator,
	resources []charmresource.Resource,
) ([]string, error) {
	args := resource.AddResourcesBeforeApplicationArgs{
		ApplicationName: appName,
		CharmLocator:    charmLocator,
		ResourceDetails: make([]resource.AddResourceDetails, len(resources)),
	}
	for i, res := range resources {
		args.ResourceDetails[i] = resource.AddResourceDetails{
			Name:   res.Name,
			Origin: res.Origin,
		}
		if res.Origin == charmresource.OriginStore {
			args.ResourceDetails[i].Revision = &res.Revision
		}
	}
	ids, err := a.resourceService.AddResourcesBeforeApplication(ctx, args)
	if err != nil {
		return nil, internalerrors.Capture(err)
	}
	returnIDs := make([]string, len(ids))
	for i, id := range ids {
		returnIDs[i] = id.String()
	}
	return returnIDs, nil
}

// updateResources handles updates of all provided resources.
func (a *API) updateResources(
	ctx context.Context,
	appID coreapplication.ID,
	resources []charmresource.Resource,
) ([]string, error) {
	newUUIDs := make([]string, len(resources))
	for i, res := range resources {
		newUUID, err := a.updateResource(ctx, appID, res)
		if errors.Is(err, resourceerrors.ArgumentNotValid) || errors.Is(err, resourceerrors.ResourceUUIDNotValid) {
			return nil, internalerrors.Errorf("%w, %w", err, errors.NotValid)
		} else if err != nil {
			// TODO: hml 2025-02-18
			// This behavior conflicts with what actually happens in the
			// juju cli where it does not make bulk calls to this method.
			// Work should be done to remove any successfully created
			// resources, to change the expected behavior.
			//
			// We don't bother aggregating errors since a partial
			// completion is disruptive and a retry of this endpoint
			// is not expensive.
			return nil, internalerrors.Capture(err)
		}
		newUUIDs[i] = newUUID.String()
	}
	return newUUIDs, nil
}

// updateResource updates the resource based on origin.
func (a *API) updateResource(
	ctx context.Context,
	appID coreapplication.ID,
	res charmresource.Resource,
) (coreresource.UUID, error) {
	resourceID, err := a.resourceService.GetApplicationResourceID(ctx,
		resource.GetApplicationResourceIDArgs{
			ApplicationID: appID,
			Name:          res.Name,
		},
	)
	if errors.Is(err, resourceerrors.ResourceNameNotValid) || errors.Is(err, resourceerrors.ResourceNotFound) {
		return "", internalerrors.Errorf("resource %q: %w", res.Name,
			err)
	} else if err != nil {
		return "", internalerrors.Capture(err)
	}

	var newUUID coreresource.UUID
	switch res.Origin {
	case charmresource.OriginStore:
		arg := resource.UpdateResourceRevisionArgs{
			ResourceUUID: resourceID,
			Revision:     res.Revision,
		}
		newUUID, err = a.resourceService.UpdateResourceRevision(ctx, arg)
	case charmresource.OriginUpload:
		newUUID, err = a.resourceService.UpdateUploadResource(ctx, resourceID)
	default:
		return "", internalerrors.Errorf("unknown origin %q", res.Origin)
	}
	return newUUID, internalerrors.Capture(err)
}

func parseApplicationTag(tagStr string) (names.ApplicationTag, *params.Error) {
	applicationTag, err := names.ParseApplicationTag(tagStr)
	if err != nil {
		return applicationTag, &params.Error{
			Message: err.Error(),
			// Note the concrete error type.
			Code: params.CodeBadRequest,
		}
	}
	return applicationTag, nil
}

func errorResult(err error) params.ResourcesResult {
	return params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		},
	}
}

func applicationResources2APIResult(svcRes coreresource.ApplicationResources) params.ResourcesResult {
	var result params.ResourcesResult
	for _, res := range svcRes.Resources {
		result.Resources = append(result.Resources, apiresources.Resource2API(res))
	}

	for _, unitResources := range svcRes.UnitResources {
		tag := names.NewUnitTag(unitResources.Name.String())
		apiRes := params.UnitResources{
			Entity: params.Entity{Tag: tag.String()},
		}
		for _, unitRes := range unitResources.Resources {
			apiRes.Resources = append(apiRes.Resources, apiresources.Resource2API(unitRes))
		}
		result.UnitResources = append(result.UnitResources, apiRes)
	}

	result.CharmStoreResources = make([]params.CharmResource, len(svcRes.RepositoryResources))
	for i, chRes := range svcRes.RepositoryResources {
		result.CharmStoreResources[i] = apiresources.CharmResource2API(chRes)
	}
	return result
}
