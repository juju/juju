// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

// Type represents the type of payload is expected from the API
type Type string

// Matches attempts to match a string to a given source.
func (t Type) Matches(o string) bool {
	return string(t) == o
}

func (t Type) String() string {
	return string(t)
}

const (
	// CharmType represents the charm payload.
	CharmType Type = "charm"
	// BundleType represents the bundle payload.
	BundleType Type = "bundle"
)

// InfoResponse is the result from an information query.
type InfoResponse struct {
	Type           Type             `json:"type"`
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Entity         Entity           `json:"result"`
	ChannelMap     []InfoChannelMap `json:"channel-map"`
	DefaultRelease InfoChannelMap   `json:"default-release,omitempty"`
	ErrorList      APIErrors        `json:"error-list,omitempty"`
}

// InfoChannelMap returns the information channel map. This defines a unique
// revision for a given channel from an info response.
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
	BundleYAML   string     `json:"bundle-yaml"`
	Platforms    []Platform `json:"platforms"`
	Revision     int        `json:"revision"`
	Version      string     `json:"version"`
}
