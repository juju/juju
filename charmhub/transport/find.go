// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type FindResponses struct {
	Results   []FindResponse `json:"results,omitempty"`
	ErrorList APIErrors      `json:"error-list,omitempty"`
}

type FindResponse struct {
	Type           Type           `json:"type"`
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Entity         Entity         `json:"result,omitempty"`
	DefaultRelease FindChannelMap `json:"default-release,omitempty"`
}

type FindChannelMap struct {
	Channel  Channel      `json:"channel,omitempty"`
	Revision FindRevision `json:"revision,omitempty"`
}

// FindRevision is different from InfoRevision.  It is missing
// ConfigYAML and MetadataYAML
type FindRevision struct {
	CreatedAt string   `json:"created-at"`
	Download  Download `json:"download"`
	Bases     []Base   `json:"bases"`
	Revision  int      `json:"revision"`
	Version   string   `json:"version"`
}
