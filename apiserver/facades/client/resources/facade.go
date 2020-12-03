// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmstore"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/resource"
	resourceapi "github.com/juju/juju/resource/api"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.resources")

// Backend is the functionality of Juju's state needed for the resources API.
type Backend interface {
	// ListResources returns the resources for the given application.
	ListResources(application string) (resource.ApplicationResources, error)

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

	factory func(chID CharmID) (NewCharmRepository, error)
}

type APIv1 struct {
	*API
}

// NewFacadeV2 creates a public API facade for resources. It is
// used for API registration.
func NewFacadeV2(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	rst, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}

	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelCfg, err := m.Config()
	if err != nil {
		return nil, errors.Trace(err)
	}

	factory := func(chID CharmID) (NewCharmRepository, error) {
		switch chID.URL.Schema {
		case "ch":
			var chCfg charmhub.Config
			chURL, ok := modelCfg.CharmHubURL()
			if ok {
				chCfg, err = charmhub.CharmHubConfigFromURL(chURL, logger.Child("client"))
			} else {
				chCfg, err = charmhub.CharmHubConfig(logger.Child("client"))
			}
			if err != nil {
				return nil, errors.Trace(err)
			}
			chClient, err := charmhub.NewClient(chCfg)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return &charmHubClient{client: chClient, id: chID}, nil
		case "cs":
			cl, err := charmstore.NewCachingClient(state.MacaroonCache{st}, controllerCfg.CharmStoreURL())
			if err != nil {
				return nil, errors.Trace(err)
			}
			return &charmStoreClient{client: cl, id: chID}, nil
		case "local":
			return &localClient{}, nil
		default:
			return nil, errors.Errorf("unrecognized charm schema %q", chID.URL.Schema)
		}
	}

	f, err := NewResourcesAPI(rst, factory)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return f, nil
}

func NewFacadeV1(ctx facade.Context) (*APIv1, error) {
	api, err := NewFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &APIv1{api}, nil
}

// NewResourcesAPI returns a new resources API facade.
func NewResourcesAPI(backend Backend, factory func(chID CharmID) (NewCharmRepository, error)) (*API, error) {
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
	}
	return f, nil
}

// ListResources returns the list of resources for the given application.
func (a *API) ListResources(args params.ListResourcesArgs) (params.ResourcesResults, error) {
	var r params.ResourcesResults
	r.Results = make([]params.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		logger.Tracef("Listing resources for %q", e.Tag)
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

		r.Results[i] = resourceapi.ApplicationResources2APIResult(svcRes)
	}
	return r, nil
}

// AddPendingResources adds the provided resources (info) to the Juju
// model in a pending state, meaning they are not available until
// resolved.  Only CharmStore and Local charms are handled, therefore
// the channel is equivalent to risk in new style channels.
func (a *APIv1) AddPendingResources(args params.AddPendingResourcesArgs) (params.AddPendingResourcesResult, error) {
	v2Args := params.AddPendingResourcesArgsV2{
		Entity: args.Entity,
		URL:    args.URL,
		CharmOrigin: params.CharmOrigin{
			Risk: args.Channel,
		},
		CharmStoreMacaroon: args.CharmStoreMacaroon,
		Resources:          args.Resources,
	}
	return a.API.AddPendingResources(v2Args)
}

// AddPendingResources adds the provided resources (info) to the Juju
// model in a pending state, meaning they are not available until
// resolved. Handles CharmHub, CharmStore and Local charms.
func (a *API) AddPendingResources(args params.AddPendingResourcesArgsV2) (params.AddPendingResourcesResult, error) {
	var result params.AddPendingResourcesResult

	tag, apiErr := parseApplicationTag(args.Tag)
	if apiErr != nil {
		result.Error = apiErr
		return result, nil
	}
	applicationID := tag.Id()

	ids, err := a.addPendingResources(applicationID, args.URL, convertParamsOrigin(args.CharmOrigin), args.Resources)
	if err != nil {
		result.Error = apiservererrors.ServerError(err)
		return result, nil
	}
	result.PendingIDs = ids
	return result, nil
}

func (a *API) addPendingResources(appName, chRef string, origin corecharm.Origin, apiResources []params.CharmResource) ([]string, error) {
	var resources []charmresource.Resource
	for _, apiRes := range apiResources {
		res, err := resourceapi.API2CharmResource(apiRes)
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
		id := CharmID{
			URL:    cURL,
			Origin: origin,
		}
		repository, err := a.factory(id)
		if err != nil {
			return nil, errors.Trace(err)
		}
		resources, err = repository.ResolveResources(resources)
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
	ApplicationTag, err := names.ParseApplicationTag(tagStr)
	if err != nil {
		return ApplicationTag, &params.Error{
			Message: err.Error(),
			Code:    params.CodeBadRequest,
		}
	}
	return ApplicationTag, nil
}

func errorResult(err error) params.ResourcesResult {
	return params.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: apiservererrors.ServerError(err),
		},
	}
}

func convertParamsOrigin(origin params.CharmOrigin) corecharm.Origin {
	var track string
	if origin.Track != nil {
		track = *origin.Track
	}
	return corecharm.Origin{
		Source:   corecharm.Source(origin.Source),
		ID:       origin.ID,
		Hash:     origin.Hash,
		Revision: origin.Revision,
		Channel: &corecharm.Channel{
			Track: track,
			Risk:  corecharm.Risk(origin.Risk),
		},
		Platform: corecharm.Platform{
			Architecture: origin.Architecture,
			OS:           origin.OS,
			Series:       origin.Series,
		},
	}
}
