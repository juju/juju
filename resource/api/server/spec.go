// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package server

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

type specState interface {
	// ListResourceSpecs returns the resource specs for the given service.
	ListResourceSpecs(service string) ([]resource.Spec, error)
}

type specFacade struct {
	state specState
}

// ListSpecs returns the list of resource specs for the given service.
func (f specFacade) ListSpecs(args api.ListSpecsArgs) (api.ListSpecsResults, error) {
	var r api.ListSpecsResults

	tag, err := names.ParseTag(args.Service)
	if err != nil {
		return r, errors.Trace(err)
	}
	service := tag.Id()

	specs, err := f.state.ListResourceSpecs(service)
	if err != nil {
		return r, errors.Trace(err)
	}

	for _, spec := range specs {
		apiSpec := api.ResourceSpec2API(spec)
		r.Results = append(r.Results, apiSpec)
	}
	return r, nil
}
