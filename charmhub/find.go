// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"strings"

	"github.com/juju/errors"
)

// FindClient defines a client for info requests.
type FindClient struct {
	path   Path
	client RESTClient
}

// NewFindClient creates a FindClient for requesting
func NewFindClient(path Path, client RESTClient) *FindClient {
	return &FindClient{
		path:   path,
		client: client,
	}
}

// Find searches Charm Hub and provides results matching a string.
func (c *FindClient) Find(ctx context.Context, query string) ([]FindResponse, error) {
	path, err := c.path.Query("q", query)
	if err != nil {
		return nil, errors.Trace(err)
	}

	var resp FindResponses
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

type FindResponses struct {
	Results   []FindResponse `json:"results"`
	ErrorList []APIError     `json:"error-list"`
}

type FindResponse struct {
	Type           string       `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Charm          Charm        `json:"charm,omitempty"`
	ChannelMap     []ChannelMap `json:"channel-map"`
	DefaultRelease ChannelMap   `json:"default-release,omitempty"`
}
