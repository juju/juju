// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
)

// specClient provides methods for interacting with resource specs
// in Juju's public RPC API.
type specClient struct {
	rawAPI
}

// ListSpecs calls the ListSpecs API server method with
// the given service name.
func (c specClient) ListSpecs(service string) ([]resource.Spec, error) {
	if service == "" {
		return nil, errors.New("missing service")
	}

	var result api.ListSpecsResults
	args := api.ListSpecsArgs{
		Service: names.NewServiceTag(service).String(),
	}
	if err := c.FacadeCall("ListSpecs", &args, &result); err != nil {
		return nil, errors.Trace(err)
	}

	specs := make([]resource.Spec, len(result.Results))
	for i, apiSpec := range result.Results {
		spec, err := api.API2ResourceSpec(apiSpec)
		if err != nil {
			// We should never see this happen; we control the input safely.
			return nil, errors.Trace(err)
		}
		specs[i] = spec
	}
	return specs, nil
}
