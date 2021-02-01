// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"net/http"
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
	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if restResp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFoundf(query)
	}
	if err := handleBasicAPIErrors(resp.ErrorList, c.logger); err != nil {
		return nil, errors.Trace(err)
	}

	return resp.Results, nil
}

// defaultFindFilter returns a filter string to retrieve all data
// necessary to fill the transport.FindResponse.  Without it, we'd
// receive the Name, ID and Type.
func defaultFindFilter() string {
	filter := defaultFindResultFilter
	filter = append(filter, appendFilterList("default-release", defaultRevisionFilter)...)
	return strings.Join(filter, ",")
}

var defaultFindResultFilter = []string{
	"result.publisher.display-name",
	"result.summary",
	"result.store-url",
}

var defaultRevisionFilter = []string{
	"revision.platforms.architecture",
	"revision.platforms.os",
	"revision.platforms.series",
	"revision.version",
}
