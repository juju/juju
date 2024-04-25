// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"context"

	"github.com/juju/charm/v13"
	charmresource "github.com/juju/charm/v13/resource"
	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiresources "github.com/juju/juju/api/client/resources"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facades/client/charms"
	corecharm "github.com/juju/juju/core/charm"
	corelogger "github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/rpc/params"
)

// Backend is the functionality of Juju's state needed for the resources API.
type Backend interface {
	// ListResources returns the resources for the given application.
	ListResources(application string) (resources.ApplicationResources, error)

	// AddPendingResource adds the resource to the data backend in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (string, error)
}

// API is the public API facade for resources.
type API struct {
	// backend is the data source for the facade.
	backend Backend

	factory func(*charm.URL) (NewCharmRepository, error)
	logger  corelogger.Logger
}

// NewFacade creates a public API facade for resources. It is
// used for API registration.
func NewFacade(ctx facade.ModelContext) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	rst := st.Resources(ctx.ObjectStore())

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelCfg, err := m.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	logger := ctx.Logger().Child("resources")
	factory := func(curl *charm.URL) (NewCharmRepository, error) {
		schema := curl.Schema
		switch {
		case charm.CharmHub.Matches(schema):
			chURL, _ := modelCfg.CharmHubURL()
			chClient, err := charmhub.NewClient(charmhub.Config{
				URL:        chURL,
				HTTPClient: ctx.HTTPClient(facade.CharmhubHTTPClient),
				Logger:     logger,
			})
			if err != nil {
				return nil, errors.Trace(err)
			}
			return repository.NewCharmHubRepository(logger.ChildWithTags("charmhub", corelogger.CHARMHUB), chClient), nil

		case charm.Local.Matches(schema):
			return &localClient{}, nil

		default:
			return nil, errors.Errorf("unrecognized charm schema %q", curl.Schema)
		}
	}

	f, err := NewResourcesAPI(rst, factory, logger)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return f, nil
}

// NewResourcesAPI returns a new resources API facade.
func NewResourcesAPI(backend Backend, factory func(*charm.URL) (NewCharmRepository, error), logger corelogger.Logger) (*API, error) {
	if backend == nil {
		return nil, errors.Errorf("missing data backend")
	}
	if factory == nil {
		// Technically this only matters for one code path through
		// AddPendingResources(). However, that functionality should be
		// provided. So we indicate the problem here instead of later
		// in the specific place where it actually matters.
		return nil, errors.Errorf("missing factory for new repository")
	}

	f := &API{
		backend: backend,
		factory: factory,
		logger:  logger,
	}
	return f, nil
}

// ListResources returns the list of resources for the given application.
func (a *API) ListResources(ctx context.Context, args params.ListResourcesArgs) (params.ResourcesResults, error) {
	var r params.ResourcesResults
	r.Results = make([]params.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		a.logger.Tracef("Listing resources for %q", e.Tag)
		tag, apierr := parseApplicationTag(e.Tag)
		if apierr != nil {
			r.Results[i] = params.ResourcesResult{
				ErrorResult: params.ErrorResult{
					Error: apierr,
				},
			}
			continue
		}

		svcRes, err := a.backend.ListResources(tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		r.Results[i] = apiresources.ApplicationResources2APIResult(svcRes)
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
		repository, err := a.factory(id.URL)
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
	userID := ""
	pendingID, err = a.backend.AddPendingResource(appName, userID, chRes)
	if err != nil {
		return "", errors.Annotatef(err, "while adding pending resource info for %q", chRes.Name)
	}
	return pendingID, nil
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
