// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// specClient provides methods for interacting with resource specs
// in Juju's public RPC API.
type specClient struct {
	FacadeCaller
}

// ListSpecs calls the ListSpecs API server method with
// the given service name.
func (c specClient) ListSpecs(services ...string) ([][]resource.Spec, error) {
	args, err := api.NewListSpecsArgs(services...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var apiResults api.SpecsResults
	if err := c.FacadeCall("ListSpecs", &args, &apiResults); err != nil {
		return nil, errors.Trace(err)
	}

	var results [][]resource.Spec
	var errs []error
	for _, apiResult := range apiResults.Results {
		errs = append(errs, apiResult.Error)
		if apiResult.Error != nil {
			results = append(results, nil)
			continue
		}

		specs := make([]resource.Spec, len(apiResult.Specs))
		for i, apiSpec := range apiResult.Specs {
			spec, err := api.API2ResourceSpec(apiSpec)
			if err != nil {
				// This could happen if the server is misbehaving
				// or non-conforming.
				return nil, errors.Trace(err)
			}
			specs[i] = spec
		}
		results = append(results, specs)
	}
	return results, nil
}
