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
	corecharm "github.com/juju/juju/core/charm"
	corehttp "github.com/juju/juju/core/http"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/repository"
	charmresource "github.com/juju/juju/internal/charm/resource"
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
		a.logger.Tracef(context.TODO(), "Listing resources for %q", e.Tag)
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

// AddPendingResources adds the provided resources (info) to the Juju
// model in a pending state, meaning they are not available until
// resolved. Handles CharmHub and Local charms.
func (a *API) AddPendingResources(ctx context.Context, args params.AddPendingResourcesArgsV2) (params.AddPendingResourcesResult, error) {
	var result params.AddPendingResourcesResult

	tag, apiErr := parseApplicationTag(args.Tag)
	if apiErr != nil {
		result.Error = apiErr
		return result, nil
	}
	applicationID := tag.Id()

	requestedOrigin, err := charms.ConvertParamsOrigin(args.CharmOrigin)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	ids, err := a.addPendingResources(ctx, applicationID, args.URL, requestedOrigin, args.Resources)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.PendingIDs = ids
	return result, nil
}

func (a *API) addPendingResources(ctx context.Context, appName, chRef string, origin corecharm.Origin, apiResources []params.CharmResource) ([]string, error) {
	var resources []charmresource.Resource
	for _, apiRes := range apiResources {
		res, err := apiresources.API2CharmResource(apiRes)
		if err != nil {
			return nil, errors.Annotatef(err, "bad resource info for %q", apiRes.Name)
		}
		resources = append(resources, res)
	}

	if chRef != "" {
		cURL, err := charm.ParseURL(chRef)
		if err != nil {
			return nil, errors.Trace(err)
		}
		id := corecharm.CharmID{
			URL:    cURL,
			Origin: origin,
		}
		repository, err := a.factory(ctx, id.URL)
		if err != nil {
			return nil, errors.Trace(err)
		}
		resources, err = repository.ResolveResources(ctx, resources, id)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}

	var ids []string
	for _, res := range resources {
		pendingID, err := a.addPendingResource(appName, res)
		if err != nil {
			// We don't bother aggregating errors since a partial
			// completion is disruptive and a retry of this endpoint
			// is not expensive.
			return nil, err
		}
		ids = append(ids, pendingID)
	}
	return ids, nil
}

func (a *API) addPendingResource(appName string, chRes charmresource.Resource) (pendingID string, err error) {
	return "", errors.Errorf("not implemented")
}

func parseApplicationTag(tagStr string) (names.ApplicationTag, *params.Error) { // note the concrete error type
	applicationTag, err := names.ParseApplicationTag(tagStr)
	if err != nil {
		return applicationTag, &params.Error{
			Message: err.Error(),
			Code:    params.CodeBadRequest,
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

func applicationResources2APIResult(svcRes resource.ApplicationResources) params.ResourcesResult {
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
