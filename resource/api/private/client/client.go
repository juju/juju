// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"
	"net/http"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/private"
)

// FacadeCaller exposes the raw API caller functionality needed here.
type FacadeCaller interface {
	// FacadeCall makes an API request.
	FacadeCall(request string, params, response interface{}) error
}

// UnitDoer exposes the raw API HTTP caller functionality needed here.
type UnitDoer interface {
	// Do sends the HTTP request/body and unpacks the response into
	// the provided "resp". If that is a **http.Response then it is
	// unpacked as-is. Otherwise it is unmarshaled from JSON.
	Do(req *http.Request, body io.ReadSeeker, resp interface{}) error
}

// NewUnitFacadeClient creates a new API client for the resources
// portion of the uniter facade.
func NewUnitFacadeClient(facadeCaller FacadeCaller, doer UnitDoer) *FacadeClient {
	return &FacadeClient{
		FacadeCaller: facadeCaller,
		doer:         doer,
	}
}

// FacadeClient is an API client for the resources portion
// of the uniter facade.
type FacadeClient struct {
	FacadeCaller
	doer UnitDoer
}

// GetResource opens the resource (metadata/blob), if it exists, via
// the HTTP API and returns it. If it does not exist or hasn't been
// uploaded yet then errors.NotFound is returned.
func (c *FacadeClient) GetResource(resourceName string) (resource.Resource, io.ReadCloser, error) {
	var response *http.Response
	req, err := api.NewHTTPDownloadRequest(resourceName)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	if err := c.doer.Do(req, nil, &response); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	// HACK(katco): Combine this into one request?
	resourceInfo, err := c.getResourceInfo(resourceName)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	// TODO(katco): Check headers against resource info
	// TODO(katco): Check in on all the response headers
	return resourceInfo, response.Body, nil
}

func (c *FacadeClient) getResourceInfo(resourceName string) (resource.Resource, error) {
	var response private.ResourcesResult

	args := private.ListResourcesArgs{
		ResourceNames: []string{resourceName},
	}
	if err := c.FacadeCall("GetResourceInfo", &args, &response); err != nil {
		return resource.Resource{}, errors.Annotate(err, "could not get resource info")
	}
	if response.Error != nil {
		err, _ := common.RestoreError(response.Error)
		return resource.Resource{}, errors.Trace(err)
	}

	if len(response.Resources) != 1 {
		return resource.Resource{}, errors.New("got bad response from API server")
	}
	if response.Resources[0].Error != nil {
		err, _ := common.RestoreError(response.Error)
		return resource.Resource{}, errors.Trace(err)
	}
	res, err := api.API2Resource(response.Resources[0].Resource)
	if err != nil {
		return resource.Resource{}, errors.Trace(err)
	}
	return res, nil
}
