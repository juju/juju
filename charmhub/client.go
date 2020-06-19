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

// Path defines a absolute path for calling requests to the server.
type Path struct {
	base *url.URL
}

// MakePath creates a URL for queries to a server.
func MakePath(base *url.URL) Path {
	return Path{
		base: base,
	}
}

// Join will sum path names onto a base URL and ensure it constructs a URL
// that is valid.
// Example:
//  - http://baseurl/name0/name1/
func (u Path) Join(names ...string) (Path, error) {
	baseURL := u.String()
	if !strings.HasSuffix(baseURL, "/") {
		baseURL += "/"
	}

	namedPath := path.Join(names...)
	path, err := url.Parse(baseURL + namedPath)
	if err != nil {
		return Path{}, errors.Trace(err)
	}
	return MakePath(path), nil
}

// String returns a stringified version of the Path.
// Under the hood this calls the url.URL#String method.
func (u Path) String() string {
	return u.base.String()
}

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

// CharmhubConfig defines a charmhub client configuration for targetting the
// snapcraft API.
func CharmhubConfig() Config {
	return Config{
		URL:     CharmhubServerURL,
		Version: CharmhubServerVersion,
		Entity:  CharmhubServerEntity,
	}
}

// BasePath returns the base configuration path for speaking to the server API.
func (c Config) BasePath() (Path, error) {
	baseURL := strings.TrimRight(c.URL, "/")
	rawURL := fmt.Sprintf("%s/%s", baseURL, path.Join(c.Version, c.Entity))
	url, err := url.Parse(rawURL)
	if err != nil {
		return Path{}, errors.Trace(err)
	}
	return MakePath(url), nil
}

// Client represents the client side of a charm store.
type Client struct {
	infoClient *InfoClient
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

	httpClient := DefaultHTTPTransport()
	apiRequester := NewAPIRequester(httpClient)
	return &Client{
		infoClient: NewInfoClient(infoPath, NewHTTPRESTClient(apiRequester)),
	}, nil
}

// Info returns charm info on the provided charm name from charmhub.
func (c *Client) Info(ctx context.Context, name string) (InfoResponse, error) {
	return c.infoClient.Get(ctx, name)
}
