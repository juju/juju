// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"bytes"
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"

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

type LatestCharmResourcesFn func(serviceId string) (resource.ServiceResources, error)

// Facade is the public API facade for resources.
type Facade struct {
	// store is the data source for the facade.
	store resourceInfoStore

	latestCharmResources LatestCharmResourcesFn
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(store DataStore, latestCharmResources LatestCharmResourcesFn) *Facade {
	return &Facade{
		store:                store,
		latestCharmResources: latestCharmResources,
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
}

// ListResources returns the list of resources for the given service.
func (f Facade) ListResources(args api.ListResourcesArgs) (api.ResourcesResults, error) {
	var r api.ResourcesResults
	r.Results = make([]api.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		logger.Tracef("Listing resources for %q", e.Tag)
		tag, apierr := parseServiceTag(e.Tag)
		if apierr != nil {
			r.Results[i] = errorResult(apierr)
			continue
		}

		svcRes, err := f.store.ListResources(tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		var result api.ResourcesResult
		for _, res := range svcRes.Resources {
			result.Resources = append(result.Resources, api.Resource2API(res))
		}
		for _, unitRes := range svcRes.UnitResources {
			unit := api.UnitResources{
				Entity: params.Entity{Tag: unitRes.Tag.String()},
			}
			for _, res := range unitRes.Resources {
				unit.Resources = append(unit.Resources, api.Resource2API(res))
			}
			result.UnitResources = append(result.UnitResources, unit)
		}

		latest, err := f.latestCharmResources(tag.Id())
		if err != nil {
			r.Results[i] = errorResult(err)
			continue
		}

		result.Updates = diffResourceVersions(svcRes.Resources, latest.Resources)

		r.Results[i] = result
	}
	return r, nil
}

func diffResourceVersions(current, available []resource.Resource) []resource.Resource {

	fingerprintEqual := func(a, b resource.Resource) bool {
		return bytes.Compare(a.Fingerprint.Bytes(), b.Fingerprint.Bytes()) == 0
	}

	var updates []resource.Resource
	for _, c := range current {
		for _, a := range available {
			if c.Name != a.Name {
				continue
			}
			if fingerprintEqual(c, a) {
				continue
			}
			updates = append(updates, a)
		}
	}

	return updates
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

	var ids []string
	for _, apiRes := range args.Resources {
		pendingID, err := f.addPendingResource(serviceID, apiRes)
		if err != nil {
			result.Error = common.ServerError(err)
			// We don't bother aggregating errors since a partial
			// completion is disruptive and a retry of this endpoint
			// is not expensive.
			return result, nil
		}
		ids = append(ids, pendingID)
	}
	result.PendingIDs = ids
	return result, nil
}

func (f Facade) addPendingResource(serviceID string, apiRes api.CharmResource) (pendingID string, err error) {
	chRes, err := api.API2CharmResource(apiRes)
	if err != nil {
		return "", errors.Annotatef(err, "bad resource info for %q", chRes.Name)
	}

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
