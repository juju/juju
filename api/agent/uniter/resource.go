// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter

import (
	"context"
	"io"
	"net/http"
	"path"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/base"
	api "github.com/juju/juju/api/client/resources"
	apihttp "github.com/juju/juju/api/http"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/resources"
	"github.com/juju/juju/rpc/params"
)

// FacadeCaller exposes the raw API caller functionality needed here.
type FacadeCaller interface {
	// FacadeCall makes an API request.
	FacadeCall(ctx context.Context, request string, args, response interface{}) error
}

// UnitHTTPClient exposes the raw API HTTP caller functionality needed here.
type UnitHTTPClient interface {
	apihttp.HTTPDoer

	// Unit Returns the name of the unit for this client.
	Unit() string
}

// NewResourcesFacadeClient creates a new API client for the resources
// portion of the uniter facade.
func NewResourcesFacadeClient(caller base.APICaller, unitTag names.UnitTag) (*ResourcesFacadeClient, error) {
	facadeCaller := base.NewFacadeCaller(caller, "ResourcesHookContext")
	httpClient, err := caller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &ResourcesFacadeClient{
		FacadeCaller: facadeCaller,
		HTTPDoer:     NewUnitHTTPClient(httpClient, unitTag.String()),
	}, nil
}

// ResourcesFacadeClient is an API client for the resources portion
// of the uniter facade.
type ResourcesFacadeClient struct {
	FacadeCaller
	apihttp.HTTPDoer
}

// GetResource opens the resource (metadata/blob), if it exists, via
// the HTTP API and returns it. If it does not exist or hasn't been
// uploaded yet then errors.NotFound is returned.
func (c *ResourcesFacadeClient) GetResource(ctx context.Context, resourceName string) (resources.Resource, io.ReadCloser, error) {
	var response *http.Response
	req, err := api.NewHTTPDownloadRequest(resourceName)
	if err != nil {
		return resources.Resource{}, nil, errors.Annotate(err, "failed to build API request")
	}
	if err := c.Do(ctx, req, &response); err != nil {
		return resources.Resource{}, nil, errors.Annotate(err, "HTTP request failed")
	}

	// HACK(katco): Combine this into one request?
	resourceInfo, err := c.getResourceInfo(resourceName)
	if err != nil {
		return resources.Resource{}, nil, errors.Trace(err)
	}

	// TODO(katco): Check headers against resource info
	// TODO(katco): Check in on all the response headers
	return resourceInfo, response.Body, nil
}

func (c *ResourcesFacadeClient) getResourceInfo(resourceName string) (resources.Resource, error) {
	var response params.UnitResourcesResult

	args := params.ListUnitResourcesArgs{
		ResourceNames: []string{resourceName},
	}
	if err := c.FacadeCall(context.TODO(), "GetResourceInfo", &args, &response); err != nil {
		return resources.Resource{}, errors.Annotate(err, "could not get resource info")
	}
	if response.Error != nil {
		err := apiservererrors.RestoreError(response.Error)
		return resources.Resource{}, errors.Annotate(err, "request failed on server")
	}

	if len(response.Resources) != 1 {
		return resources.Resource{}, errors.New("got bad response from API server")
	}
	if response.Resources[0].Error != nil {
		err := apiservererrors.RestoreError(response.Error)
		return resources.Resource{}, errors.Annotate(err, "request failed for resource")
	}
	res, err := api.API2Resource(response.Resources[0].Resource)
	if err != nil {
		return resources.Resource{}, errors.Annotate(err, "got bad data from API server")
	}
	return res, nil
}

type unitHTTPClient struct {
	apihttp.HTTPDoer
	unitName string
}

// NewUnitHTTPClient wraps an HTTP client (a la httprequest.Client)
// with unit information. This allows rewriting of the URL to match
// the relevant unit.
func NewUnitHTTPClient(client apihttp.HTTPDoer, unitName string) UnitHTTPClient {
	return &unitHTTPClient{
		HTTPDoer: client,
		unitName: unitName,
	}
}

// Unit returns the name of the unit.
func (uhc unitHTTPClient) Unit() string {
	return uhc.unitName
}

// Do implements httprequest.Doer.
func (uhc *unitHTTPClient) Do(ctx context.Context, req *http.Request, response interface{}) error {
	req.URL.Path = path.Join("/units", uhc.unitName, req.URL.Path)
	return uhc.HTTPDoer.Do(ctx, req, response)
}
