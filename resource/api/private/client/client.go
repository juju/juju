// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"io"
	"net/http"
	"path"

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

// HTTPClient exposes the raw API HTTP caller functionality needed here.
type HTTPClient interface {
	// Do sends the HTTP request/body and unpacks the response into
	// the provided "resp". If that is a **http.Response then it is
	// unpacked as-is. Otherwise it is unmarshaled from JSON.
	Do(req *http.Request, body io.ReadSeeker, resp interface{}) error
}

// UnitHTTPClient exposes the raw API HTTP caller functionality needed here.
type UnitHTTPClient interface {
	HTTPClient

	// Unit Returns the name of the unit for this client.
	Unit() string
}

// NewUnitFacadeClient creates a new API client for the resources
// portion of the uniter facade.
func NewUnitFacadeClient(facadeCaller FacadeCaller, httpClient UnitHTTPClient) *UnitFacadeClient {
	return &UnitFacadeClient{
		FacadeCaller: facadeCaller,
		HTTPClient:   httpClient,
	}
}

// UnitFacadeClient is an API client for the resources portion
// of the uniter facade.
type UnitFacadeClient struct {
	FacadeCaller
	HTTPClient
}

// GetResource opens the resource (metadata/blob), if it exists, via
// the HTTP API and returns it. If it does not exist or hasn't been
// uploaded yet then errors.NotFound is returned.
func (c *UnitFacadeClient) GetResource(resourceName string) (resource.Resource, io.ReadCloser, error) {
	var response *http.Response
	req, err := api.NewHTTPDownloadRequest(resourceName)
	if err != nil {
		return resource.Resource{}, nil, errors.Annotate(err, "failed to build API request")
	}
	if err := c.Do(req, nil, &response); err != nil {
		return resource.Resource{}, nil, errors.Annotate(err, "HTTP request failed")
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

func (c *UnitFacadeClient) getResourceInfo(resourceName string) (resource.Resource, error) {
	var response private.ResourcesResult

	args := private.ListResourcesArgs{
		ResourceNames: []string{resourceName},
	}
	if err := c.FacadeCall("GetResourceInfo", &args, &response); err != nil {
		return resource.Resource{}, errors.Annotate(err, "could not get resource info")
	}
	if response.Error != nil {
		err := common.RestoreError(response.Error)
		return resource.Resource{}, errors.Annotate(err, "request failed on server")
	}

	if len(response.Resources) != 1 {
		return resource.Resource{}, errors.New("got bad response from API server")
	}
	if response.Resources[0].Error != nil {
		err := common.RestoreError(response.Error)
		return resource.Resource{}, errors.Annotate(err, "request failed for resource")
	}
	res, err := api.API2Resource(response.Resources[0].Resource)
	if err != nil {
		return resource.Resource{}, errors.Annotate(err, "got bad data from API server")
	}
	return res, nil
}

type unitHTTPClient struct {
	HTTPClient
	unitName string
}

// NewUnitHTTPClient wraps an HTTP client (a la httprequest.Client)
// with unit information. This allows rewriting of the URL to match
// the relevant unit.
func NewUnitHTTPClient(client HTTPClient, unitName string) UnitHTTPClient {
	return &unitHTTPClient{
		HTTPClient: client,
		unitName:   unitName,
	}
}

// Unit returns the name of the unit.
func (uhc unitHTTPClient) Unit() string {
	return uhc.unitName
}

// Do implements httprequest.Doer.
func (uhc *unitHTTPClient) Do(req *http.Request, body io.ReadSeeker, response interface{}) error {
	req.URL.Path = path.Join("/units", uhc.unitName, req.URL.Path)
	return uhc.HTTPClient.Do(req, body, response)
}
