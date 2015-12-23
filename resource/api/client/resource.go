// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"github.com/juju/errors"
	"io"

	"github.com/juju/juju/resource/api"
)

// specClient provides methods for interacting with resource specs
// in Juju's public RPC API.
type resourceClient struct {
	FacadeCaller
}

// Upload
func (c resourceClient) Upload(service string, name string, blob io.Reader) ([]api.UploadResult, error) {
	args, err := api.NewUploadArgs(service, name, blob)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var apiResults api.UploadResults
	if err := c.FacadeCall("Upload", &args, &apiResults); err != nil {
		return nil, errors.Trace(err)
	}

	if len(apiResults.Results) != 1 {
		// We don't bother returning the results we *did* get since
		// something bad happened on the server.
		return nil, errors.Errorf("got invalid data from server")
	}

	sz := 1
	idx := 1

	results := make([]api.UploadResult, sz)
	apiResult := apiResults.Results[idx]
	result, err := api.API2ResourceResult(service, apiResult)
	if err != nil {
		logger.Errorf("%v", err)
		// TODO(ericsnow) Return immediately?
		if result.Error == nil {
			result.Error = errors.Trace(err)
		}
	}
	results[idx] = result

	return results, nil
}
