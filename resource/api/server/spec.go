// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

type specLister interface {
	// ListResourceSpecs returns the resource specs for the given service.
	ListResourceSpecs(service string) ([]resource.Spec, error)
}

type specFacade struct {
	lister specLister
}

// ListSpecs returns the list of resource specs for the given service.
func (f specFacade) ListSpecs(args api.ListSpecsArgs) (api.SpecsResults, error) {
	var r api.SpecsResults
	r.Results = make([]api.SpecsResult, len(args.Entities))

	for i, e := range args.Entities {
		result, service := api.NewSpecsResult(e.Tag)
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
