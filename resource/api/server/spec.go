// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// specLister is the portion of Juju's "state" needed for specFacade.
type specLister interface {
	// ListResourceSpecs returns the resource specs for the given service.
	ListResourceSpecs(service string) ([]resource.Spec, error)
}

// specFacade is the portion of the resources facade dealing
// with resource specs.
type specFacade struct {
	// lister is the data source for the facade.
	lister specLister
}

// ListSpecs returns the list of resource specs for the given service.
func (f specFacade) ListSpecs(args api.ListSpecsArgs) (api.ResourceSpecsResults, error) {
	var r api.ResourceSpecsResults
	r.Results = make([]api.ResourceSpecsResult, len(args.Entities))

	for i, e := range args.Entities {
		result, service := api.NewResourceSpecsResult(e.Tag)
		r.Results[i] = result
		if result.Error != nil {
			continue
		}

		specs, err := f.lister.ListResourceSpecs(service)
		if err != nil {
			api.SetResultError(&r.Results[i], err)
			continue
		}

		var apiSpecs []api.ResourceSpec
		for _, spec := range specs {
			apiSpec := api.ResourceSpec2API(spec)
			apiSpecs = append(apiSpecs, apiSpec)
		}
		r.Results[i].Specs = apiSpecs
	}
	return r, nil
}
