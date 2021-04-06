// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

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
	Channel  Channel      `json:"channel,omitempty"`
	Revision InfoRevision `json:"revision,omitempty"`
}

// InfoRevision is different from FindRevision.  It has additional
// fields of ConfigYAML and MetadataYAML.
// NOTE: InfoRevision will be filled in with the CharmHub api response
// differently within the InfoResponse.ChannelMap and the
// InfoResponse.DefaultRelease.  The DefaultRelease InfoRevision will
// include ConfigYAML, MetadataYAML and BundleYAML
// NOTE 2: actions-yaml is a possible response for the DefaultRelease
// InfoRevision, but not implemented.
type InfoRevision struct {
	ConfigYAML string `json:"config-yaml"`
	CreatedAt  string `json:"created-at"`
	// Via filters, only Download.Size will be available.
	Download     Download `json:"download"`
	MetadataYAML string   `json:"metadata-yaml"`
	BundleYAML   string   `json:"bundle-yaml"`
	Bases        []Base   `json:"bases"`
	Revision     int      `json:"revision"`
	Version      string   `json:"version"`
}
