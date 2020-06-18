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

// Get requests the information of a given charm. If that charm doesn't exist
// an error stating that fact will be returned.
func (i *InfoClient) Get(ctx context.Context, name string) (InfoResponse, error) {
	var resp InfoResponse
	path, err := i.path.Join(name)
	if err != nil {
		return resp, errors.Trace(err)
	}

	if err := i.client.Get(ctx, path, &resp); err != nil {
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

// TODO: (hml) 2020-06-17
// Why do we fail unmarshalling to this structure?
type Media struct {
	Height int    `json:"height"`
	Type   string `json:"type"`
	URL    string `json:"url"`
	Width  int    `json:"width"`
}
