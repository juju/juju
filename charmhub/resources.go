// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"
	"github.com/kr/pretty"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

// ResourcesClient defines a client for resources requests.
type ResourcesClient struct {
	path   path.Path
	client RESTClient
	logger Logger
}

// NewResourcesClient creates a ResourcesClient for requesting
func NewResourcesClient(path path.Path, client RESTClient, logger Logger) *ResourcesClient {
	return &ResourcesClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

func (c *ResourcesClient) ListResourceRevisions(ctx context.Context, charm, resource string) (transport.ResourcesResponse, error) {
	c.logger.Tracef("ListResourceRevisions(%s, %s)", charm, resource)
	var resp transport.ResourcesResponse
	path, err := c.path.Join(charm, resource, "revisions")
	if err != nil {
		return resp, errors.Trace(err)
	}
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return resp, errors.Trace(err)
	}

	c.logger.Tracef("ListResourceRevisions(%s, %s) unmarshalled: %s", charm, resource, pretty.Sprint(resp))
	return resp, nil
}
