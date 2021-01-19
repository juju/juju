// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/http"

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

// ListResourceRevisions returns a slice of resource revisions for the provided
// resource of the given charm.
func (c *ResourcesClient) ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error) {
	c.logger.Tracef("ListResourceRevisions(%s, %s)", charm, resource)
	var resp transport.ResourcesResponse
	path, err := c.path.Join(charm, resource, "revisions")
	if err != nil {
		return nil, errors.Trace(err)
	}
	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf("%q for %q", charm, resource)
	}
	c.logger.Tracef("ListResourceRevisions(%s, %s) unmarshalled: %s", charm, resource, pretty.Sprint(resp.Revisions))
	return resp.Revisions, nil
}

var resourceFilter = []string{
	"download.hash-sha-384",
	"download.size",
	"download.url",
	"name",
	"revision",
	"filename",
	"description",
	"type",
}
