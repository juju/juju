// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

var logger = loggo.GetLogger("juju.resource.api.server")

const (
	// Version is the version number of the current Facade.
	Version = 1
)

// DataStore is the functionality of Juju's state needed for the resources API.
type DataStore interface {
	resourceInfoStore
	UploadDataStore
}

// CharmStore exposes the functionality of the charm store as needed here.
type CharmStore interface {
	// ListResources composes, for each of the identified charms, the
	// list of details for each of the charm's resources. Those details
	// are those associated with the specific charm revision. They
	// include the resource's metadata and revision.
	ListResources(charmURLs []*charm.URL) ([][]charmresource.Resource, error)

	// GetResource returns a reader for the resource's data. That data
	// is streamed from the charm store. The charm's revision, if any,
	// is ignored. If the identified resource is not in the charm store
	// then errors.NotFound is returned.
	GetResource(cURL *charm.URL, resourceName string, revision int) (charmresource.Resource, io.ReadCloser, error)
}

// Facade is the public API facade for resources.
type Facade struct {
	// store is the data source for the facade.
	store resourceInfoStore

	newCharmstoreClient func(*charm.URL, *macaroon.Macaroon) (CharmStore, error)
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(store DataStore, newClient func(*charm.URL, *macaroon.Macaroon) (CharmStore, error)) (*Facade, error) {
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

// resourceInfoStore is the portion of Juju's "state" needed
// for the resources facade.
type resourceInfoStore interface {
	// ListResources returns the resources for the given service.
	ListResources(service string) (resource.ServiceResources, error)

	// AddPendingResource adds the resource to the data store in a
	// "pending" state. It will stay pending (and unavailable) until
	// it is resolved. The returned ID is used to identify the pending
	// resources when resolving it.
	AddPendingResource(serviceID, userID string, chRes charmresource.Resource, r io.Reader) (string, error)
}

// ListResources returns the list of resources for the given service.
func (f Facade) ListResources(args api.ListResourcesArgs) (api.ResourcesResults, error) {
	var r api.ResourcesResults
	r.Results = make([]api.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		logger.Tracef("Listing resources for %q", e.Tag)
		tag, apierr := parseServiceTag(e.Tag)
		if apierr != nil {
			r.Results[i] = api.ResourcesResult{
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

		r.Results[i] = api.ServiceResources2APIResult(svcRes)
	}
	return r, nil
}

// AddPendingResources adds the provided resources (info) to the Juju
// model in a pending state, meaning they are not available until
// resolved.
func (f Facade) AddPendingResources(args api.AddPendingResourcesArgs) (api.AddPendingResourcesResult, error) {
	var result api.AddPendingResourcesResult

	tag, apiErr := parseServiceTag(args.Tag)
	if apiErr != nil {
		result.Error = apiErr
		return result, nil
	}
	serviceID := tag.Id()

	ids, err := f.addPendingResources(serviceID, args.URL, args.CharmStoreMacaroon, args.Resources)
	if err != nil {
		result.Error = common.ServerError(err)
		return result, nil
	}
	result.PendingIDs = ids
	return result, nil
}

func (f Facade) addPendingResources(serviceID, chRef string, csMac *macaroon.Macaroon, apiResources []api.CharmResource) ([]string, error) {
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
			resources, err = f.resolveCharmstoreResources(cURL, csMac, resources)
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
		pendingID, err := f.addPendingResource(serviceID, res)
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

func (f Facade) resolveCharmstoreResources(cURL *charm.URL, csMac *macaroon.Macaroon, resources []charmresource.Resource) ([]charmresource.Resource, error) {
	client, err := f.newCharmstoreClient(cURL, csMac)
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources, err := f.resourcesFromCharmstore(cURL, client)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveResources(resources, storeResources, cURL, client)
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
func (f Facade) resourcesFromCharmstore(cURL *charm.URL, client CharmStore) (map[string]charmresource.Resource, error) {
	results, err := client.ListResources([]*charm.URL{cURL})
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
func resolveResources(resources []charmresource.Resource, storeResources map[string]charmresource.Resource, cURL *charm.URL, client CharmStore) ([]charmresource.Resource, error) {
	allResolved := make([]charmresource.Resource, len(resources))
	copy(allResolved, resources)
	for i, res := range resources {
		// Note that incoming "upload" resources take precedence over
		// ones already known to the controller, regardless of their
		// origin.
		if res.Origin != charmresource.OriginStore {
			continue
		}

		resolved, err := resolveStoreResource(res, storeResources, cURL, client)
		if err != nil {
			return nil, errors.Trace(err)
		}
		allResolved[i] = resolved
	}
	return allResolved, nil
}

// resolveStoreResource selects the resource info to use. It decides
// between the provided and latest info based on the revision.
func resolveStoreResource(res charmresource.Resource, storeResources map[string]charmresource.Resource, cURL *charm.URL, client CharmStore) (charmresource.Resource, error) {
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
		storeRes, r, err := client.GetResource(cURL, res.Name, res.Revision)
		if err != nil {
			return storeRes, errors.Trace(err)
		}
		r.Close() // We don't care about the file.
		return storeRes, nil
	}
	// The caller fully-specified a resource with a different resource
	// revision than the one associated with the charm in the store. So
	// we use the provided info as-is.
	return res, nil
}

func (f Facade) addPendingResource(serviceID string, chRes charmresource.Resource) (pendingID string, err error) {
	userID := ""
	var reader io.Reader
	pendingID, err = f.store.AddPendingResource(serviceID, userID, chRes, reader)
	if err != nil {
		return "", errors.Annotatef(err, "while adding pending resource info for %q", chRes.Name)
	}
	return pendingID, nil
}

func parseServiceTag(tagStr string) (names.ServiceTag, *params.Error) { // note the concrete error type
	serviceTag, err := names.ParseServiceTag(tagStr)
	if err != nil {
		return serviceTag, &params.Error{
			Message: err.Error(),
			Code:    params.CodeBadRequest,
		}
	}
	return serviceTag, nil
}

func errorResult(err error) api.ResourcesResult {
	return api.ResourcesResult{
		ErrorResult: params.ErrorResult{
			Error: common.ServerError(err),
		},
	}
}
