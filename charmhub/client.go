// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// CharmHub is a client for communication with charmHub.  Unlike
// the charmHub client within juju, this package does not rely on
// wrapping an external package client. Generic client code for this
// package has been copied from "github.com/juju/charmrepo/v6/csclient".
//
// TODO: (hml) 2020-06-17
// Implement:
// - use of macaroons, at that time consider refactoring the local
//   charmHub pkg to share macaroonJar.
// - user/password ?
// - allow for use of the channel pieces

package charmhub

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"

	charmhubpath "github.com/juju/juju/charmhub/path"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/version"
)

// ServerURL holds the default location of the global charm hub.
// An alternate location can be configured by changing the URL
// field in the Params struct.
const (
	CharmHubServerURL     = "https://api.snapcraft.io"
	CharmHubServerVersion = "v2"
	CharmHubServerEntity  = "charms"
)

var (
	userAgentKey   = "User-Agent"
	userAgentValue = "Juju/" + version.Current.String()
)

const defaultMinMultipartUploadSize = 5 * 1024 * 1024

// Config holds configuration for creating a new charm hub client.
type Config struct {
	// URL holds the base endpoint URL of the charmHub,
	// with no trailing slash, not including the version.
	// For example https://api.snapcraft.io/v2/charms/
	URL string

	// Version holds the version attribute of the charmHub we're requesting.
	Version string

	// Entity holds the entity to target when querying the API (charm or snaps).
	Entity string

	// Headers allow the defining of a set of default headers when sending the
	// requests. These headers augment the headers required for sending requests
	// and allow overriding existing headers.
	Headers http.Header
}

// CharmHubConfig defines a charmHub client configuration for targeting the
// snapcraft API.
func CharmHubConfig() Config {
	return CharmHubConfigFromURL(CharmHubServerURL)
}

// CharmHubConfigFromURL defines a charmHub client configuration with a given
// URL for targeting the API.
func CharmHubConfigFromURL(url string) Config {
	// By default we want to specify a default user-agent here. In the future
	// we should ensure this probably contains model UUID and cloud.
	headers := make(http.Header)
	headers.Set(userAgentKey, userAgentValue)

	return Config{
		URL:     url,
		Version: CharmHubServerVersion,
		Entity:  CharmHubServerEntity,
		Headers: headers,
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
	url            string
	infoClient     *InfoClient
	findClient     *FindClient
	downloadClient *DownloadClient
	refreshClient  *RefreshClient
}

// NewClient creates a new charmHub client from the supplied configuration.
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

	refreshPath, err := base.Join("refresh")
	if err != nil {
		return nil, errors.Annotate(err, "constructing refresh path")
	}

	httpClient := DefaultHTTPTransport()
	apiRequester := NewAPIRequester(httpClient)
	restClient := NewHTTPRESTClient(apiRequester, config.Headers)
	fileSystem := DefaultFileSystem()

	return &Client{
		url:           base.String(),
		infoClient:    NewInfoClient(infoPath, restClient),
		findClient:    NewFindClient(findPath, restClient),
		refreshClient: NewRefreshClient(refreshPath, restClient),
		// download client doesn't require a path here, as the download could
		// be from any server in theory. That information is found from the
		// refresh response.
		downloadClient: NewDownloadClient(httpClient, fileSystem),
	}, nil
}

// URL returns the underlying store URL.
func (c *Client) URL() string {
	return c.url
}

// Info returns charm info on the provided charm name from CharmHub API.
func (c *Client) Info(ctx context.Context, name string) (transport.InfoResponse, error) {
	return c.infoClient.Info(ctx, name)
}

// Find searches for a given charm for a given name from CharmHub API.
func (c *Client) Find(ctx context.Context, name string) ([]transport.FindResponse, error) {
	return c.findClient.Find(ctx, name)
}

// Refresh defines a client for making refresh API calls, that allow for
// updating a series of charms to the latest version.
func (c *Client) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	return c.refreshClient.Refresh(ctx, config)
}

// Download defines a client for downloading charms directly.
func (c *Client) Download(ctx context.Context, resourceURL *url.URL, archivePath string) (*charm.CharmArchive, error) {
	return c.downloadClient.Download(ctx, resourceURL, archivePath)
}
