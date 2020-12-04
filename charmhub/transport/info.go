// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type Type = string

type InfoResponse struct {
	Type           Type             `json:"type"`
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Entity         Entity           `json:"result"`
	ChannelMap     []InfoChannelMap `json:"channel-map"`
	DefaultRelease InfoChannelMap   `json:"default-release,omitempty"`
	ErrorList      APIErrors        `json:"error-list,omitempty"`
}

type InfoChannelMap struct {
	Channel   Channel            `json:"channel,omitempty"`
	Resources []ResourceRevision `json:"resources,omitempty"`
	Revision  InfoRevision       `json:"revision,omitempty"`
}

// InfoRevision is different from FindRevision.  It has additional
// fields of ConfigYAML and MetadataYAML.
type InfoRevision struct {
	ConfigYAML   string     `json:"config-yaml"`
	CreatedAt    string     `json:"created-at"`
	Download     Download   `json:"download"`
	MetadataYAML string     `json:"metadata-yaml"`
	Platforms    []Platform `json:"platforms"`
	Revision     int        `json:"revision"`
	Version      string     `json:"version"`
}
