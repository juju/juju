// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"

	"github.com/juju/errors"
)

// InfoClient defines a client for info requests.
type InfoClient struct {
	path   Path
	client RESTClient
}

// NewInfoClient creates a InfoClient for requesting
func NewInfoClient(path Path, client RESTClient) *InfoClient {
	return &InfoClient{
		path:   path,
		client: client,
	}
}

// Info requests the information of a given charm. If that charm doesn't exist
// an error stating that fact will be returned.
func (c *InfoClient) Info(ctx context.Context, name string) (InfoResponse, error) {
	var resp InfoResponse
	path, err := c.path.Join(name)
	if err != nil {
		return resp, errors.Trace(err)
	}

	if err := c.client.Get(ctx, path, &resp); err != nil {
		return resp, errors.Trace(err)
	}
	return resp, nil
}

type InfoResponse struct {
	Type           string       `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Charm          Charm        `json:"charm,omitempty"`
	ChannelMap     []ChannelMap `json:"channel-map"`
	DefaultRelease ChannelMap   `json:"default-release,omitempty"`
}
