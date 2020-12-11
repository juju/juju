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

	var fields string
	if query != "" {
		fields = defaultFindFilter()
	} else {
		fields = subsetFindFilter()
	}

	path, err = path.Query("fields", fields)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resp transport.FindResponses
	restResp, err := c.client.Get(ctx, path, &resp)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if resultErr := resp.ErrorList.Combine(); resultErr != nil {
		if restResp.StatusCode == http.StatusNotFound {
			return nil, errors.NewNotFound(resultErr, "")
		}
		return nil, resultErr
	}

	return resp.Results, nil
}

// defaultFindFilter returns a filter string to retrieve all data
// necessary to fill the transport.FindResponse.  Without it, we'd
// receive the Name, ID and Type.
func defaultFindFilter() string {
	filter := defaultResultFilter
	filter = append(filter, appendFilterList("default-release.revision", defaultDownloadFilter)...)
	filter = append(filter, appendFilterList("default-release", findRevisionFilter)...)
	filter = append(filter, appendFilterList("default-release", defaultChannelFilter)...)
	return strings.Join(filter, ",")
}

// subsetFindFilter returns a filter subset for all the data we need for a large
// search.
func subsetFindFilter() string {
	filter := subsetResultFilter
	filter = append(filter, appendFilterList("default-release", subsetRevisionFilter)...)
	return strings.Join(filter, ",")
}

var findRevisionFilter = []string{
	"revision.created-at",
	"revision.platforms.architecture",
	"revision.platforms.os",
	"revision.platforms.series",
	"revision.revision",
	"revision.version",
}

var subsetResultFilter = []string{
	"result.publisher.display-name",
	"result.summary",
}

var subsetRevisionFilter = []string{
	"revision.platforms.series",
	"revision.version",
}
