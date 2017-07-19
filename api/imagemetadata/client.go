// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata

import (
	"github.com/juju/errors"

	"github.com/juju/juju/api/base"
)

// Client provides access to cloud image metadata.
// It is used to update published image metadata.
type Client struct {
	base.ClientFacade
	facade base.FacadeCaller
}

// NewClient returns a new metadata client.
func NewClient(st base.APICallCloser) *Client {
	frontend, backend := base.NewClientFacade(st, "ImageMetadata")
	return &Client{ClientFacade: frontend, facade: backend}
}

// UpdateFromPublishedImages retrieves currently published image metadata and
// updates stored ones accordingly.
// This method is primarily intended for a worker.
func (c *Client) UpdateFromPublishedImages() error {
	return errors.Trace(
		c.facade.FacadeCall("UpdateFromPublishedImages", nil, nil))
}
