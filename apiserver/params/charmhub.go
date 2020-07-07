// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// Query holds the query information when attempting to find possible charms or
// bundles for searching the charmhub.
type Query struct {
	Query string `json:"query"`
}

// TODO (hml) 2020-06-17
// Create actual params.InfoResponse and params.ErrorResponse structs for use
// here.
type CharmHubEntityInfoResult struct {
	Result InfoResponse  `json:"result"`
	Errors ErrorResponse `json:"errors"`
}

type InfoResponse struct {
	Type           string         `json:"type"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Entity         CharmHubEntity `json:"entity"`
	ChannelMap     []ChannelMap   `json:"channel-map"`
	DefaultRelease ChannelMap     `json:"default-release,omitempty"`
}

type CharmHubEntityFindResult struct {
	Results []FindResponse `json:"result"`
	Errors  ErrorResponse  `json:"errors"`
}

type FindResponse struct {
	Type           string         `json:"type"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Entity         CharmHubEntity `json:"entity"`
	ChannelMap     []ChannelMap   `json:"channel-map"`
	DefaultRelease ChannelMap     `json:"default-release,omitempty"`
}

type ChannelMap struct {
	Channel  Channel  `json:"channel,omitempty"`
	Revision Revision `json:"revision,omitempty"`
}

type Channel struct {
	Name       string   `json:"name"` // track/risk
	Platform   Platform `json:"platform"`
	ReleasedAt string   `json:"released-at"`
}

type Platform struct {
	Architecture string `json:"architecture"`
	OS           string `json:"os"`
	Series       string `json:"series"`
}

type Revision struct {
	CreatedAt    string     `json:"created-at"`
	ConfigYAML   string     `json:"config-yaml"`
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

type CharmHubEntity struct {
	Categories  []Category        `json:"categories"`
	Description string            `json:"description"`
	License     string            `json:"license"`
	Media       []Media           `json:"media"`
	Publisher   map[string]string `json:"publisher"`
	Summary     string            `json:"summary"`
	UsedBy      []string          `json:"used-by"` // bundles which use the charm
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

type ErrorResponse struct {
	Error CharmHubError `json:"error-list"`
}

type CharmHubError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
