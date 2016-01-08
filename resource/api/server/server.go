// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/loggo"

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
	ListResources(service string) ([]resource.Resource, error)
}

// ListResources returns the list of resources for the given service.
func (f Facade) ListResources(args api.ListResourcesArgs) (api.ResourcesResults, error) {
	var r api.ResourcesResults
	r.Results = make([]api.ResourcesResult, len(args.Entities))

	for i, e := range args.Entities {
		logger.Tracef("Listing resources for %q", e.Tag)
		result, service := api.NewResourcesResult(e.Tag)
		r.Results[i] = result
		if result.Error != nil {
			continue
		}

		resources, err := f.lister.ListResources(service)
		if err != nil {
			api.SetResultError(&r.Results[i], err)
			continue
		}

		var apiResources []api.Resource
		for _, res := range resources {
			apiRes := api.Resource2API(res)
			apiResources = append(apiResources, apiRes)
		}
		r.Results[i].Resources = apiResources
	}
	return r, nil
}
