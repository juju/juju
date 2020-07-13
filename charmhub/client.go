// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// CharmHub is a client for communication with charmhub.  Unlike
// the charmhub client within juju, this package does not rely on
// wrapping an external package client. Generic client code for this
// package has been copied from "github.com/juju/charmrepo/v5/csclient".
//
// TODO: (hml) 2020-06-17
// Implement:
// - use of macaroons, at that time consider refactoring the local
//   charmhub pkg to share macaroonJar.
// - user/password ?
// - allow for use of the channel pieces

package charmhub

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"github.com/juju/errors"

	charmhubpath "github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
)

// ServerURL holds the default location of the global charm hub.
// An alternate location can be configured by changing the URL
// field in the Params struct.
const (
	CharmhubServerURL     = "https://api.snapcraft.io"
	CharmhubServerVersion = "v2"
	CharmhubServerEntity  = "charms"
)

const (
	userAgentKey   = "User-Agent"
	userAgentValue = "Golang_CSClient/4.0"
)

const defaultMinMultipartUploadSize = 5 * 1024 * 1024

// Config holds configuration for creating a new charm hub client.
type Config struct {
	// URL holds the base endpoint URL of the charmhub,
	// with no trailing slash, not including the version.
	// For example https://api.snapcraft.io/v2/charms/
	URL string

	// Version holds the version attribute of the charmhub we're requesting.
	Version string

	// Entity holds the entity to target when querying the API (charm or snaps).
	Entity string
}

// CharmhubConfig defines a charmhub client configuration for targeting the
// snapcraft API.
func CharmhubConfig() Config {
	return Config{
		URL:     CharmhubServerURL,
		Version: CharmhubServerVersion,
		Entity:  CharmhubServerEntity,
	}
}

// BasePath returns the base configuration path for speaking to the server API.
func (c Config) BasePath() (charmhubpath.Path, error) {
	baseURL := strings.TrimRight(c.URL, "/")
	rawURL := fmt.Sprintf("%s/%s", baseURL, path.Join(c.Version, c.Entity))
	url, err := url.Parse(rawURL)
	if err != nil {
		return charmhubpath.Path{}, errors.Trace(err)
	}
	return charmhubpath.MakePath(url), nil
}

// Client represents the client side of a charm store.
type Client struct {
	url        string
	infoClient *InfoClient
	findClient *FindClient
}

// NewClient creates a new charmhub client from the supplied configuration.
func NewClient(config Config) (*Client, error) {
	base, err := config.BasePath()
	if err != nil {
		return nil, errors.Trace(err)
	}

	infoPath, err := base.Join("info")
	if err != nil {
		return nil, errors.Annotate(err, "constructing info path")
	}

	findPath, err := base.Join("find")
	if err != nil {
		return nil, errors.Annotate(err, "constructing find path")
	}

	httpClient := DefaultHTTPTransport()
	apiRequester := NewAPIRequester(httpClient)
	restClient := NewHTTPRESTClient(apiRequester)

	return &Client{
		url:        base.String(),
		infoClient: NewInfoClient(infoPath, restClient),
		findClient: NewFindClient(findPath, restClient),
	}, nil
}

// URL returns the underlying store URL.
func (c *Client) URL() string {
	return c.url
}

// Info returns charm info on the provided charm name from charmhub.
func (c *Client) Info(ctx context.Context, name string) (transport.InfoResponse, error) {
	return c.infoClient.Info(ctx, name)
}

// Find searches for a given charm for a given name from charmhub.
func (c *Client) Find(ctx context.Context, name string) ([]transport.FindResponse, error) {
	return c.findClient.Find(ctx, name)
}
