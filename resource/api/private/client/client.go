package client

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/juju/resource"
)

type facadeCaller interface{}

func NewUnitFacadeClient(facadeCaller facadeCaller) *FacadeClient {
	return &FacadeClient{}
}

type FacadeClient struct{}

func (c *FacadeClient) GetResourceInfo(resourceName string) (resource.Resource, error) {
	return resource.Resource{}, errors.NotImplementedf("")
}

func (c *FacadeClient) GetResourceDownloader(resourceName string) (io.Reader, error) {
	return nil, errors.NotImplementedf("")
}
