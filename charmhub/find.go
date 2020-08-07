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
}

// NewFindClient creates a FindClient for querying charm or bundle information.
func NewFindClient(path path.Path, client RESTClient) *FindClient {
	return &FindClient{
		path:   path,
		client: client,
	}
}

// Find searches Charm Hub and provides results matching a string.
func (c *FindClient) Find(ctx context.Context, query string) ([]transport.FindResponse, error) {
	path, err := c.path.Query("q", query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resp transport.FindResponses
	if err := c.client.Get(ctx, path, &resp); err != nil {
		return nil, errors.Trace(err)
	}

	if len(resp.ErrorList) > 0 {
		var combined []string
		for _, err := range resp.ErrorList {
			if err.Message != "" {
				combined = append(combined, err.Message)
			}
		}
		return nil, errors.Errorf(strings.Join(combined, "\n"))
	}

	return resp.Results, nil
}
