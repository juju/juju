// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

// InfoClient defines a client for info requests.
type InfoClient struct {
	path   path.Path
	client RESTClient
}

// NewInfoClient creates a InfoClient for requesting
func NewInfoClient(path path.Path, client RESTClient) *InfoClient {
	return &InfoClient{
		path:   path,
		client: client,
	}
}

// Info requests the information of a given charm. If that charm doesn't exist
// an error stating that fact will be returned.
func (c *InfoClient) Info(ctx context.Context, name string) (transport.InfoResponse, error) {
	var resp transport.InfoResponse
	path, err := c.path.Join(name)
	if err != nil {
		return resp, errors.Trace(err)
	}

	if err := c.client.Get(ctx, path, &resp); err != nil {
		return resp, errors.Trace(err)
	}

	return resp, resp.ErrorList.Combine()
}
