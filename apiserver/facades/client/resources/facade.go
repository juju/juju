// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"github.com/juju/charm/v7"
	charmresource "github.com/juju/charm/v7/resource"
	csparams "github.com/juju/charmrepo/v5/csclient/params"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.resources")

// Backend is the functionality of Juju's state needed for the resources API.
type Backend interface {
	// ListResources returns the resources for the given application.
	ListResources(service string) (resource.ApplicationResources, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(applicationID, userID string, chRes charmresource.Resource) (string, error)
}

// CharmStore exposes the functionality of the charm store as needed here.
type CharmStore interface {
	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources([]charmstore.CharmID) ([][]charmresource.Resource, error)

	// ResourceInfo returns the metadata for the given resource.
	ResourceInfo(charmstore.ResourceRequest) (charmresource.Resource, error)
}

// Facade is the public API facade for resources.
type Facade struct {
	// store is the data source for the facade.
	store Backend

	newCharmstoreClient func() (CharmStore, error)
}

// NewPublicFacade creates a public API facade for resources. It is
// used for API registration.
func NewPublicFacade(st *state.State, _ facade.Resources, authorizer facade.Authorizer) (*Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	rst, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerCfg, err := st.ControllerConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	newClient := func() (CharmStore, error) {
		return charmstore.NewCachingClient(state.MacaroonCache{st}, controllerCfg.CharmStoreURL())
	}
	facade, err := NewFacade(rst, newClient)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}

// NewFacade returns a new resoures API facade.
func NewFacade(store Backend, newClient func() (CharmStore, error)) (*Facade, error) {
	if store == nil {
		return nil, errors.Errorf("missing data store")
	}
	if newClient == nil {
		// Technically this only matters for one code path through
		// AddPendingResources(). However, that functionality should be
		// provided. So we indicate the problem here instead of later
		// in the specific place where it actually matters.
		return nil, errors.Errorf("missing factory for new charm store clients")
	}

	f := &Facade{
		store:               store,
		newCharmstoreClient: newClient,
	}
	return f, nil
}

// ListResources returns the list of resources for the given application.
func (f Facade) ListResources(args params.ListResourcesArgs) (params.ResourcesResults, error) {
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

		svcRes, err := f.store.ListResources(tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		r.Results[i] = api.ApplicationResources2APIResult(svcRes)
	}
	return r, nil
}

// AddPendingResources adds the provided resources (info) to the Juju
// model in a pending state, meaning they are not available until
// resolved.
func (f Facade) AddPendingResources(args params.AddPendingResourcesArgs) (params.AddPendingResourcesResult, error) {
	var result params.AddPendingResourcesResult

	tag, apiErr := parseApplicationTag(args.Tag)
	if apiErr != nil {
		result.Error = apiErr
		return result, nil
	}
	applicationID := tag.Id()

	channel := csparams.Channel(args.Channel)
	ids, err := f.addPendingResources(applicationID, args.URL, channel, args.CharmStoreMacaroon, args.Resources)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	result.PendingIDs = ids
	return result, nil
}

func (f Facade) addPendingResources(applicationID, chRef string, channel csparams.Channel, csMac *macaroon.Macaroon, apiResources []params.CharmResource) ([]string, error) {
	var resources []charmresource.Resource
	for _, apiRes := range apiResources {
		res, err := api.API2CharmResource(apiRes)
		if err != nil {
			return nil, errors.Annotatef(err, "bad resource info for %q", apiRes.Name)
		}
		resources = append(resources, res)
	}

	if chRef != "" {
		cURL, err := charm.ParseURL(chRef)
		if err != nil {
			return nil, err
		}

		switch cURL.Schema {
		case "cs":
			id := charmstore.CharmID{
				URL:     cURL,
				Channel: channel,
			}
			resources, err = f.resolveCharmstoreResources(id, csMac, resources)
			if err != nil {
				return nil, errors.Trace(err)
			}
		case "local":
			resources, err = f.resolveLocalResources(resources)
			if err != nil {
				return nil, errors.Trace(err)
			}
		default:
			return nil, errors.Errorf("unrecognized charm schema %q", cURL.Schema)
		}
	}

	var ids []string
	for _, res := range resources {
		pendingID, err := f.addPendingResource(applicationID, res)
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

func (f Facade) resolveCharmstoreResources(id charmstore.CharmID, csMac *macaroon.Macaroon, resources []charmresource.Resource) ([]charmresource.Resource, error) {
	client, err := f.newCharmstoreClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	ids := []charmstore.CharmID{id}
	storeResources, err := f.resourcesFromCharmstore(ids, client)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveResources(resources, storeResources, id, client)
	if err != nil {
		return nil, err
	}
	// TODO(ericsnow) Ensure that the non-upload resource revisions
	// match a previously published revision set?
	return resolved, nil
}

func (f Facade) resolveLocalResources(resources []charmresource.Resource) ([]charmresource.Resource, error) {
	var resolved []charmresource.Resource
	for _, res := range resources {
		resolved = append(resolved, charmresource.Resource{
			Meta:   res.Meta,
			Origin: charmresource.OriginUpload,
		})
	}
	return resolved, nil
}

// resourcesFromCharmstore gets the info for the charm's resources in
// the charm store. If the charm URL has a revision then that revision's
// resources are returned. Otherwise the latest info for each of the
// resources is returned.
func (f Facade) resourcesFromCharmstore(charms []charmstore.CharmID, client CharmStore) (map[string]charmresource.Resource, error) {
	results, err := client.ListResources(charms)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources := make(map[string]charmresource.Resource)
	if len(results) != 0 {
		for _, res := range results[0] {
			storeResources[res.Name] = res
		}
	}
	return storeResources, nil
}

// resolveResources determines the resource info that should actually
// be stored on the controller. That decision is based on the provided
// resources along with those in the charm store (if any).
func resolveResources(resources []charmresource.Resource, storeResources map[string]charmresource.Resource, id charmstore.CharmID, client CharmStore) ([]charmresource.Resource, error) {
	allResolved := make([]charmresource.Resource, len(resources))
	copy(allResolved, resources)
	for i, res := range resources {
		// Note that incoming "upload" resources take precedence over
		// ones already known to the controller, regardless of their
		// origin.
		if res.Origin != charmresource.OriginStore {
			continue
		}

		resolved, err := resolveStoreResource(res, storeResources, id, client)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allResolved[i] = resolved
	}
	return allResolved, nil
}

// resolveStoreResource selects the resource info to use. It decides
// between the provided and latest info based on the revision.
func resolveStoreResource(res charmresource.Resource, storeResources map[string]charmresource.Resource, id charmstore.CharmID, client CharmStore) (charmresource.Resource, error) {
	storeRes, ok := storeResources[res.Name]
	if !ok {
		// This indicates that AddPendingResources() was called for
		// a resource the charm store doesn't know about (for the
		// relevant charm revision).
		// TODO(ericsnow) Do the following once the charm store supports
		// the necessary endpoints:
		// return res, errors.NotFoundf("charm store resource %q", res.Name)
		return res, nil
	}

	if res.Revision < 0 {
		// The caller wants to use the charm store info.
		return storeRes, nil
	}
	if res.Revision == storeRes.Revision {
		// We don't worry about if they otherwise match. Only the
		// revision is significant here. So we use the info from the
		// charm store since it is authoritative.
		return storeRes, nil
	}
	if res.Fingerprint.IsZero() {
		// The caller wants resource info from the charm store, but with
		// a different resource revision than the one associated with
		// the charm in the store.
		req := charmstore.ResourceRequest{
			Charm:    id.URL,
			Channel:  id.Channel,
			Name:     res.Name,
			Revision: res.Revision,
		}
		storeRes, err := client.ResourceInfo(req)
		if err != nil {
			return storeRes, errors.Trace(err)
		}
		return storeRes, nil
	}
	// The caller fully-specified a resource with a different resource
	// revision than the one associated with the charm in the store. So
	// we use the provided info as-is.
	return res, nil
}

func (f Facade) addPendingResource(applicationID string, chRes charmresource.Resource) (pendingID string, err error) {
	userID := ""
	pendingID, err = f.store.AddPendingResource(applicationID, userID, chRes)
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
			Error: common.ServerError(err),
		},
	}
}
