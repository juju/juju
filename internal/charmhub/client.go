// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// CharmHub is a client for communication with charmHub.  Unlike
// the charmHub client within juju, this package does not rely on
// wrapping an external package client. Generic client code for this
// package has been copied from "github.com/juju/charmrepo/v7/csclient".
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
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/juju/errors"

	corelogger "github.com/juju/juju/core/logger"
	charmhubpath "github.com/juju/juju/internal/charmhub/path"
	"github.com/juju/juju/internal/charmhub/transport"
)

const (
	// DefaultServerURL is the default location of the global Charmhub API.
	// An alternate location can be configured by changing the URL
	// field in the Config struct.
	DefaultServerURL = "https://api.charmhub.io"

	// RefreshTimeout is the timout callers should use for Refresh calls.
	RefreshTimeout = 10 * time.Second
)

const (
	serverVersion = "v2"
	serverEntity  = "charms"
)

// Config holds configuration for creating a new charm hub client.
// The zero value is a valid default configuration.
type Config struct {
	// Logger to use during the API requests. This field is required.
	Logger corelogger.Logger

	// URL holds the base endpoint URL of the Charmhub API,
	// with no trailing slash, not including the version.
	// If empty string, use the default Charmhub API server.
	URL string

	// HTTPClient represents the HTTP client to use for all API
	// requests. If nil, use the default HTTP client.
	HTTPClient HTTPClient

	// FileSystem represents the file system operations for downloading.
	// If nil, use the real OS file system.
	// This is only required for downloading of charms or bundles.
	FileSystem FileSystem
}

// basePath returns the base configuration path for speaking to the server API.
func basePath(configURL string) (charmhubpath.Path, error) {
	baseURL := strings.TrimRight(configURL, "/")
	rawURL := fmt.Sprintf("%s/%s", baseURL, path.Join(serverVersion, serverEntity))
	url, err := url.Parse(rawURL)
	if err != nil {
		return charmhubpath.Path{}, errors.Trace(err)
	}
	return charmhubpath.MakePath(url), nil
}

// Client represents the client side of a charm store.
type Client struct {
	url             string
	infoClient      *infoClient
	findClient      *findClient
	downloadClient  *DownloadClient
	refreshClient   *refreshClient
	resourcesClient *resourcesClient
}

// NewClient creates a new Charmhub client from the supplied configuration.
func NewClient(config Config) (*Client, error) {
	logger := config.Logger.Child("client", corelogger.CHARMHUB)

	url := config.URL
	if url == "" {
		url = DefaultServerURL
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = DefaultHTTPClient(logger)
	}

	fs := config.FileSystem
	if fs == nil {
		fs = fileSystem{}
	}

	base, err := basePath(url)
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

	resourcesPath, err := base.Join("resources")
	if err != nil {
		return nil, errors.Annotate(err, "constructing resources path")
	}

	apiRequester := newAPIRequester(httpClient, logger)
	apiRequestLogger := newAPIRequesterLogger(apiRequester, logger)
	restClient := newHTTPRESTClient(apiRequestLogger)

	return &Client{
		url:             base.String(),
		infoClient:      newInfoClient(infoPath, restClient, logger),
		findClient:      newFindClient(findPath, restClient, logger),
		refreshClient:   newRefreshClient(refreshPath, restClient, logger),
		resourcesClient: newResourcesClient(resourcesPath, restClient, logger),

		// download client doesn't require a path here, as the download could
		// be from any server in theory. That information is found from the
		// refresh response.
		downloadClient: NewDownloadClient(httpClient, fs, logger),
	}, nil
}

// URL returns the underlying store URL.
func (c *Client) URL() string {
	return c.url
}

// Info returns charm info on the provided charm name from CharmHub API.
func (c *Client) Info(ctx context.Context, name string, options ...InfoOption) (transport.InfoResponse, error) {
	return c.infoClient.Info(ctx, name, options...)
}

// Find searches for a given charm for a given name from CharmHub API.
func (c *Client) Find(ctx context.Context, name string, options ...FindOption) ([]transport.FindResponse, error) {
	return c.findClient.Find(ctx, name, options...)
}

// Refresh defines a client for making refresh API calls with different actions.
func (c *Client) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	return c.refreshClient.Refresh(ctx, config)
}

// RefreshWithRequestMetrics defines a client for making refresh API calls.
// Specifically to use the refresh action and provide metrics.  Intended for
// use in the charm revision updater facade only.  Otherwise use Refresh.
func (c *Client) RefreshWithRequestMetrics(ctx context.Context, config RefreshConfig, metrics Metrics) ([]transport.RefreshResponse, error) {
	return c.refreshClient.RefreshWithRequestMetrics(ctx, config, metrics)
}

// RefreshWithMetricsOnly defines a client making a refresh API call with no
// action, whose purpose is to send metrics data for models without current
// units.  E.G. the controller model.
func (c *Client) RefreshWithMetricsOnly(ctx context.Context, metrics Metrics) error {
	return c.refreshClient.RefreshWithMetricsOnly(ctx, metrics)
}

// Download defines a client for downloading charms directly.
func (c *Client) Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*Digest, error) {
	return c.downloadClient.Download(ctx, resourceURL, archivePath, options...)
}

// ListResourceRevisions returns resource revisions for the provided charm and resource.
func (c *Client) ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error) {
	return c.resourcesClient.ListResourceRevisions(ctx, charm, resource)
}
