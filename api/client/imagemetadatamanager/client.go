// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadatamanager

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/rpc/params"
)

// Option is a function that can be used to configure a Client.
type Option = base.Option

// WithTracer returns an Option that configures the Client to use the
// supplied tracer.
var WithTracer = base.WithTracer

// Client provides access to cloud image metadata.
// It is used to find, save and update image metadata.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new metadata client.
func NewClient(st base.APICallCloser, options ...Option) *Client {
	frontend, backend := base.NewClientFacade(st, "ImageMetadataManager", options...)
	return &Client{ClientFacade: frontend, facade: backend}
}

// List returns image metadata that matches filter.
// Empty filter will return all image metadata.
func (c *Client) List(
	stream, region string,
	bases []corebase.Base, arches []string,
	virtType, rootStorageType string,
) ([]params.CloudImageMetadata, error) {
	versions := make([]string, len(bases))
	for i, s := range bases {
		versions[i] = s.Channel.Track
	}
	in := params.ImageMetadataFilter{
		Region:          region,
		Versions:        versions,
		Arches:          arches,
		Stream:          stream,
		VirtType:        virtType,
		RootStorageType: rootStorageType,
	}
	out := params.ListCloudImageMetadataResult{}
	err := c.facade.FacadeCall(context.TODO(), "List", in, &out)
	return out.Result, err
}

// Save saves specified image metadata.
// Supports bulk saves for scenarios like cloud image metadata caching at bootstrap.
func (c *Client) Save(metadata []params.CloudImageMetadata) error {
	in := params.MetadataSaveParams{
		Metadata: []params.CloudImageMetadataList{{metadata}},
	}
	out := params.ErrorResults{}
	err := c.facade.FacadeCall(context.TODO(), "Save", in, &out)
	if err != nil {
		return errors.Trace(err)
	}
	if len(out.Results) != 1 {
		return errors.Errorf("exected 1 result, got %d", len(out.Results))
	}
	if out.Results[0].Error != nil {
		return errors.Trace(out.Results[0].Error)
	}
	return nil
}

// Delete removes image metadata for given image id from stored metadata.
func (c *Client) Delete(imageId string) error {
	in := params.MetadataImageIds{[]string{imageId}}
	out := params.ErrorResults{}
	err := c.facade.FacadeCall(context.TODO(), "Delete", in, &out)
	if err != nil {
		return errors.Trace(err)
	}

	result := out.Results
	if len(result) != 1 {
		return errors.Errorf("expected to find one result for image id %q but found %d", imageId, len(result))
	}

	theOne := result[0]
	if theOne.Error != nil {
		return errors.Trace(theOne.Error)
	}
	return nil
}
