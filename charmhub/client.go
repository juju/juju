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
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

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
	CharmHubServerURL     = "https://api.charmhub.io"
	CharmHubServerVersion = "v2"
	CharmHubServerEntity  = "charms"

	MetadataHeader = "X-Juju-Metadata"

	RefreshTimeout = 10 * time.Second
)

var (
	userAgentKey   = "User-Agent"
	userAgentValue = version.UserAgentVersion
)

// Logger is a in place interface to represent a logger for consuming.
type Logger interface {
	IsTraceEnabled() bool

	Errorf(string, ...interface{})
	Debugf(string, ...interface{})
	Tracef(string, ...interface{})
}

// Config holds configuration for creating a new charm hub client.
type Config struct {
	// URL holds the base endpoint URL of the charmHub,
	// with no trailing slash, not including the version.
	// For example https://api.charmhub.io/v2/charms/
	URL string

	// Version holds the version attribute of the charmHub we're requesting.
	Version string

	// Entity holds the entity to target when querying the API (charm or snaps).
	Entity string

	// Headers allow the defining of a set of default headers when sending the
	// requests. These headers augment the headers required for sending requests
	// and allow overriding existing headers.
	Headers http.Header

	// Transport represents the default http transport to use for all API
	// requests.
	Transport Transport

	// Logger to use during the API requests.
	Logger Logger
}

// Option to be passed into charmhub construction to customize the client.
type Option func(*options)

type options struct {
	url             *string
	metadataHeaders map[string]string
	transportFunc   func(Logger) Transport
}

// WithURL sets the url on the option.
func WithURL(u string) Option {
	return func(options *options) {
		options.url = &u
	}
}

// WithMetadataHeaders sets the headers on the option.
func WithMetadataHeaders(h map[string]string) Option {
	return func(options *options) {
		options.metadataHeaders = h
	}
}

// WithHTTPTransport sets the default http transport to use on the option.
func WithHTTPTransport(transportFn func(Logger) Transport) Option {
	return func(options *options) {
		options.transportFunc = transportFn
	}
}

// Create a options instance with default values.
func newOptions() *options {
	u := CharmHubServerURL
	return &options{
		url: &u,
		transportFunc: func(logger Logger) Transport {
			return DefaultHTTPTransport(logger)
		},
	}
}

// CharmHubConfig defines a charmHub client configuration for targeting the
// charmhub API.
func CharmHubConfig(logger Logger, options ...Option) (Config, error) {
	opts := newOptions()
	for _, option := range options {
		option(opts)
	}

	if logger == nil {
		return Config{}, errors.NotValidf("nil logger")
	}

	// By default we want to specify a default user-agent here. In the future
	// we should ensure this probably contains model UUID and cloud.
	headers := make(http.Header)
	headers.Set(userAgentKey, userAgentValue)

	// Additionally apply any metadata headers to the headers so we can send
	// every time we make a request.
	m := opts.metadataHeaders
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		headers.Add(MetadataHeader, k+"="+m[k])
	}

	return Config{
		URL:       *opts.url,
		Version:   CharmHubServerVersion,
		Entity:    CharmHubServerEntity,
		Headers:   headers,
		Transport: opts.transportFunc(logger),
		Logger:    logger,
	}, nil
}

// CharmHubConfigFromURL defines a charmHub client configuration with a given
// URL for targeting the API.
func CharmHubConfigFromURL(url string, logger Logger, options ...Option) (Config, error) {
	return CharmHubConfig(logger, append(options, WithURL(url))...)
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
	url             string
	infoClient      *InfoClient
	findClient      *FindClient
	downloadClient  *DownloadClient
	refreshClient   *RefreshClient
	resourcesClient *ResourcesClient
	logger          Logger
}

// NewClient creates a new charmHub client from the supplied configuration.
func NewClient(config Config) (*Client, error) {
	fileSystem := DefaultFileSystem()
	return NewClientWithFileSystem(config, fileSystem)
}

// NewClientWithFileSystem creates a new charmHub client from the supplied
// configuration and a file system.
func NewClientWithFileSystem(config Config, fileSystem FileSystem) (*Client, error) {
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

	resourcesPath, err := base.Join("resources")
	if err != nil {
		return nil, errors.Annotate(err, "constructing resources path")
	}

	config.Logger.Tracef("NewClient to %q", config.URL)

	apiRequester := NewAPIRequester(config.Transport, config.Logger)
	restClient := NewHTTPRESTClient(apiRequester, config.Headers)

	return &Client{
		url:           base.String(),
		infoClient:    NewInfoClient(infoPath, restClient, config.Logger),
		findClient:    NewFindClient(findPath, restClient, config.Logger),
		refreshClient: NewRefreshClient(refreshPath, restClient, config.Logger),
		// download client doesn't require a path here, as the download could
		// be from any server in theory. That information is found from the
		// refresh response.
		downloadClient:  NewDownloadClient(config.Transport, fileSystem, config.Logger),
		resourcesClient: NewResourcesClient(resourcesPath, restClient, config.Logger),
		logger:          config.Logger,
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

// Refresh defines a client for making refresh API calls, that allow for
// updating a series of charms to the latest version.
func (c *Client) Refresh(ctx context.Context, config RefreshConfig) ([]transport.RefreshResponse, error) {
	return c.refreshClient.Refresh(ctx, config)
}

// Download defines a client for downloading charms directly.
func (c *Client) Download(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) error {
	return c.downloadClient.Download(ctx, resourceURL, archivePath, options...)
}

// DownloadAndRead defines a client for downloading charms directly.
func (c *Client) DownloadAndRead(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (*charm.CharmArchive, error) {
	return c.downloadClient.DownloadAndRead(ctx, resourceURL, archivePath, options...)
}

// DownloadAndReadBundle defines a client for downloading bundles directly.
func (c *Client) DownloadAndReadBundle(ctx context.Context, resourceURL *url.URL, archivePath string, options ...DownloadOption) (charm.Bundle, error) {
	return c.downloadClient.DownloadAndReadBundle(ctx, resourceURL, archivePath, options...)
}

// DownloadResource returns an io.ReadCloser to read the Resource from.
func (c *Client) DownloadResource(ctx context.Context, resourceURL *url.URL) (r io.ReadCloser, err error) {
	return c.downloadClient.DownloadResource(ctx, resourceURL)
}

// ListResourceRevisions returns resource revisions for the provided charm and resource.
func (c *Client) ListResourceRevisions(ctx context.Context, charm, resource string) ([]transport.ResourceRevision, error) {
	return c.resourcesClient.ListResourceRevisions(ctx, charm, resource)
}
