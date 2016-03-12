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
}

// Facade is the public API facade for resources.
type Facade struct {
	// store is the data source for the facade.
	store resourceInfoStore

	newCharmstoreClient func(*charm.URL, *macaroon.Macaroon) (CharmStore, error)
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(store DataStore, newClient func(*charm.URL, *macaroon.Macaroon) (CharmStore, error)) *Facade {
	return &Facade{
		store:               store,
		newCharmstoreClient: newClient,
	}
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

	// Units returns the tags for all units in the given service.
	Units(serviceID string) (units []names.UnitTag, err error)
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

		units, err := f.store.Units(tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		r.Results[i] = api.ServiceResources2APIResult(svcRes, units)
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
		orig := apiRes.Revision
		if apiRes.Revision < 0 {
			apiRes.Revision = 0
		}
		res, err := api.API2CharmResource(apiRes)
		if err != nil {
			return nil, errors.Annotatef(err, "bad resource info for %q", apiRes.Name)
		}
		res.Revision = orig
		resources = append(resources, res)
	}

	if chRef != "" {
		cURL, err := charm.ParseURL(chRef)
		if err != nil {
			return nil, err
		}
		storeResources, err := f.resourcesFromCharmstore(cURL, csMac, resources)
		if err != nil {
			return nil, err
		}
		resources = storeResources
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

func (f Facade) resourcesFromCharmstore(cURL *charm.URL, csMac *macaroon.Macaroon, resources []charmresource.Resource) ([]charmresource.Resource, error) {
	if f.newCharmstoreClient == nil {
		return nil, errors.NotSupportedf("could not get resource info from charm store")
	}

	client, err := f.newCharmstoreClient(cURL, csMac)
	if err != nil {
		return nil, errors.Trace(err)
	}
	results, err := client.ListResources([]*charm.URL{cURL})
	if err != nil {
		return nil, errors.Trace(err)
	}
	storeResources := make(map[string]charmresource.Resource)
	for _, res := range results[0] {
		storeResources[res.Name] = res
	}

	combined := make([]charmresource.Resource, len(resources))
	copy(combined, resources)
	for i, res := range resources {
		if res.Origin != charmresource.OriginStore {
			continue
		}
		storeRes, ok := storeResources[res.Name]
		if !ok {
			// TODO(ericsnow) Fail instead?
			continue
		}
		revision := neededRevision(res, storeRes)
		if revision < 0 {
			// The resource info is already okay.
			// TODO(ericsnow) Verify that the info is correct via a
			// client.GetResource() call?
			continue
		}
		if revision != storeRes.Revision {
			// We have a desired revision but are missing the
			// fingerprint, etc.
			// TODO(ericsnow) Call client.GetResource() to get info for that revision.
			return nil, errors.NotSupportedf("could not get resource info")
		}
		// Otherwise we use the info from the store as-is.
		combined[i] = storeRes
	}

	return combined, nil
}

func neededRevision(res, latest charmresource.Resource) int {
	if res.Revision < 0 {
		return latest.Revision
	}
	if res.Revision == latest.Revision {
		return latest.Revision
	}
	if res.Fingerprint.IsZero() {
		return res.Revision
	}
	return -1 // use it as-is
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
