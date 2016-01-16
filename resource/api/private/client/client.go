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

type FacadeCaller interface {
	FacadeCall(request string, params, response interface{}) error
}

// Doer
type UnitDoer interface {
	Do(req *http.Request, body io.ReadSeeker, resp interface{}) error
}

func NewUnitFacadeClient(facadeCaller FacadeCaller, doer UnitDoer) *FacadeClient {
	return &FacadeClient{
		FacadeCaller: facadeCaller,
		doer:         doer,
	}
}

type FacadeClient struct {
	FacadeCaller
	doer UnitDoer
}

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
