// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
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

// Facade is the public API facade for resources.
type Facade struct {
	// store is the data source for the facade.
	store resourceInfoStore
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(store DataStore) *Facade {
	return &Facade{
		store: store,
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
