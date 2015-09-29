// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/params"
)

var logger = loggo.GetLogger("juju.api.imagemetadata")

// Client provides access to cloud image metadata.
// It is used to find, save and update image metadata.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new metadata client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ImageMetadata")
	return &Client{ClientFacade: frontend, facade: backend}
}

// List returns image metadata that matches filter.
// Empty filter will return all image metadata.
func (c *Client) List(
	stream, region string,
	series, arches []string,
	virtType, rootStorageType string,
) ([]params.CloudImageMetadata, error) {
	in := params.ImageMetadataFilter{
		Region:          region,
		Series:          series,
		Arches:          arches,
		Stream:          stream,
		VirtType:        virtType,
		RootStorageType: rootStorageType,
	}
	out := params.ListCloudImageMetadataResult{}
	err := c.facade.FacadeCall("List", in, &out)
	return out.Result, err
}

// Save saves specified image metadata.
// Supports bulk saves for scenarios like cloud image metadata caching at bootstrap.
func (c *Client) Save(metadata []params.CloudImageMetadata) ([]params.ErrorResult, error) {
	in := params.MetadataSaveParams{Metadata: metadata}
	out := params.ErrorResults{}
	err := c.facade.FacadeCall("Save", in, &out)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return out.Results, nil
}

// UpdateFromPublishedImages retrieves currently published image metadata and
// updates stored ones accordingly.
// This method is primarily intended for a worker.
func (c *Client) UpdateFromPublishedImages() error {
	return errors.Trace(
		c.facade.FacadeCall("UpdateFromPublishedImages", nil, nil))
}
