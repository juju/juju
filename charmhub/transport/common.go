// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

// The following contains all the common DTOs for a gathering information from
// a given store.

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

type Entity struct {
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
