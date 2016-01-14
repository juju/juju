package client

import (
	"fmt"
	"io"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
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
		return nil, errors.Annotate(err, "could not get resource info")
	}
	return response, nil
}

func (c *FacadeClient) GetResourceDownloader(resourceName string) (io.ReadCloser, error) {
	var response *http.Response
	url := fmt.Sprintf("/resources/%s", resourceName)
	req := http.NewRequest("GET", url, nil)
	if err := c.doer.Do(req, nil, response); err != nil {
		return nil, errors.Trace(err)
	}

	return response.Body, nil
}
