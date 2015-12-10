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
func (c specClient) ListSpecs(services []string) ([]resource.SpecsResult, error) {
	args, err := api.NewListSpecsArgs(services...)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var apiResults api.ResourceSpecsResults
	if err := c.FacadeCall("ListSpecs", &args, &apiResults); err != nil {
		return nil, errors.Trace(err)
	}

	if len(apiResults.Results) != len(services) {
		// We don't bother returning the results we *did* get since
		// something bad happened on the server.
		return nil, errors.Errorf("got invalid data from server (expected %d results, got %d)", len(services), len(apiResults.Results))
	}

	results := make([]resource.SpecsResult, len(services))
	for i, service := range services {
		apiResult := apiResults.Results[i]

		result, err := api.API2SpecsResult(service, apiResult)
		if err != nil {
			logger.Errorf("%v", err)
			// TODO(ericsnow) Return immediately?
			if result.Error == nil {
				result.Error = errors.Trace(err)
			}
		}
		results[i] = result
	}

	return results, nil
}
