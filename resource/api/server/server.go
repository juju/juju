// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/loggo"
	"github.com/juju/names"

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
	resourceLister
	UploadDataStore
}

// Facade is the public API facade for resources.
type Facade struct {
	// lister is the data source for the ListResources endpoint.
	lister resourceLister
}

// NewFacade returns a new resoures facade for the given Juju state.
func NewFacade(data DataStore) *Facade {
	return &Facade{
		lister: data,
	}
}

// resourceLister is the portion of Juju's "state" needed
// for the ListResources endpoint.
type resourceLister interface {
	// ListResources returns the resources for the given service.
	ListResources(service string) (resource.ServiceResources, error)
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

		svcRes, err := f.lister.ListResources(tag.Id())
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
		r.Results = append(r.Results, result)
	}
	return r, nil
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
