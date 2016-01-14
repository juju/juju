package client

import (
	"io"
	"net/http"

	"github.com/juju/errors"
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

func (c *FacadeClient) GetResourceInfo(resourceName string) (resource.Resource, error) {
	var response resource.Resource
	args := private.GetResourceInfoArgs{
		ResourceName: resourceName,
	}
	if err := c.FacadeCall("GetResourceInfo", &args, &response); err != nil {
		return resource.Resource{}, errors.Annotate(err, "could not get resource info")
	}
	return response, nil
}

func (c *FacadeClient) GetResource(resourceName string) (resource.Resource, io.ReadCloser, error) {
	var response *http.Response
	req, err := api.NewHTTPDownloadRequest(resourceName)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}
	if err := c.doer.Do(req, nil, response); err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	// HACK(katco): Combine this into one request?
	resourceInfo, err := c.GetResourceInfo(resourceName)
	if err != nil {
		return resource.Resource{}, nil, errors.Trace(err)
	}

	// TODO(katco): Check headers against resource info
	// TODO(katco): Check in on all the response headers
	return resourceInfo, response.Body, nil
}
