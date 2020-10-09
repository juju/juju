// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

// FindClient defines a client for querying information about a given charm or
// bundle for a given CharmHub store.
type FindClient struct {
	path   path.Path
	client RESTClient
	logger Logger
}

// NewFindClient creates a FindClient for querying charm or bundle information.
func NewFindClient(path path.Path, client RESTClient, logger Logger) *FindClient {
	return &FindClient{
		path:   path,
		client: client,
		logger: logger,
	}
}

// Find searches Charm Hub and provides results matching a string.
func (c *FindClient) Find(ctx context.Context, query string) ([]transport.FindResponse, error) {
	c.logger.Tracef("Find(%s)", query)
	path, err := c.path.Query("q", query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	path, err = path.Query("fields", defaultFindFilter())
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resp transport.FindResponses
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	return resp.Results, resp.ErrorList.Combine()
}

// defaultFindFilter returns a filter string to retrieve all data
// necessary to fill the transport.FindResponse.  Without it, we'd
// receive the Name, ID and Type.
func defaultFindFilter() string {
	filter := defaultResultFilter
	filter = append(filter, appendFilterList("default-release.revision", defaultDownloadFilter)...)
	filter = append(filter, appendFilterList("default-release", defaultRevisionFilter)...)
	filter = append(filter, appendFilterList("default-release", defaultChannelFilter)...)
	return strings.Join(filter, ",")
}
