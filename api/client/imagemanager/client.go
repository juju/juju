// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemanager

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

// Client provides access to the imagemanager, used to list/delete images.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new imagemanager client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ImageManager")
	return &Client{ClientFacade: frontend, facade: backend}
}

// ListImages returns the images.
func (c *Client) ListImages(kind, series, arch string) ([]params.ImageMetadata, error) {
	p := params.ImageFilterParams{
		Images: []params.ImageSpec{
			{Kind: kind, Series: series, Arch: arch},
		},
	}
	var result params.ListImageResult
	err := c.facade.FacadeCall("ListImages", p, &result)
	return result.Result, err
}

// DeleteImage deletes the specified image.
func (c *Client) DeleteImage(kind, series, arch string) error {
	p := params.ImageFilterParams{
		Images: []params.ImageSpec{
			{Kind: kind, Series: series, Arch: arch},
		},
	}
	results := new(params.ErrorResults)
	err := c.facade.FacadeCall("DeleteImages", p, results)
	if err != nil {
		return err
	}
	return results.OneError()
}
