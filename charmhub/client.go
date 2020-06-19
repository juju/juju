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

	var (
		httpClient   = DefaultHTTPTransport()
		apiRequester = NewAPIRequester(httpClient)
		restClient   = NewHTTPRESTClient(apiRequester)
	)
	return &Client{
		infoClient: NewInfoClient(infoPath, restClient),
		findClient: NewFindClient(findPath, restClient),
	}, nil
}

// Info returns charm info on the provided charm name from charmhub.
func (c *Client) Info(ctx context.Context, name string) (InfoResponse, error) {
	return c.infoClient.Info(ctx, name)
}

// Find searches for a given charm for a given name from charmhub.
func (c *Client) Find(ctx context.Context, name string) ([]FindResponse, error) {
	return c.findClient.Find(ctx, name)
}

type ChannelMap struct {
	Channel  Channel  `json:"channel,omitempty"`
	Revision Revision `json:"revision,omitempty"`
}

type Channel struct {
	Name       string   `json:"name"`
	Platform   Platform `json:"platform"`
	ReleasedAt string   `json:"released-at"`
	Risk       string   `json:"risk"`
	Track      string   `json:"track"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Series       string `json:"series"`
}

type Revision struct {
	ConfigYAML   string     `json:"config-yaml"`
	CreatedAt    string     `json:"created-at"`
	Download     Download   `json:"download"`
	MetadataYAML string     `json:"metadata-yaml"`
	Platforms    []Platform `json:"platforms"`
	Revision     int        `json:"revision"`
	Version      string     `json:"version"`
}

type Download struct {
	HashSHA265 string `json:"hash-sha-265"`
	Size       int    `json:"size"`
	URL        string `json:"url"`
}

type Charm struct {
	Categories  []Category        `json:"categories"`
	Description string            `json:"description"`
	License     string            `json:"license"`
	Media       []Media           `json:"media"`
	Publisher   map[string]string `json:"publisher"`
	Summary     string            `json:"summary"`
	UsedBy      []string          `json:"used-by"`
}

type Category struct {
	Featured bool   `json:"featured"`
	Name     string `json:"name"`
}

type Media struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}
