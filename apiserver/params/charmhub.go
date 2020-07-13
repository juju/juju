// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// Query holds the query information when attempting to find possible charms or
// bundles for searching the charmhub.
type Query struct {
	Query string `json:"query"`
}

type CharmHubEntityInfoResult struct {
	Result InfoResponse  `json:"result"`
	Errors ErrorResponse `json:"errors"`
}

type InfoResponse struct {
	Type        string             `json:"type"`
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Publisher   string             `json:"publisher"`
	Summary     string             `json:"summary"`
	Series      []string           `json:"series"`
	StoreURL    string             `json:"store-url"`
	Tags        []string           `json:"tags"`
	Charm       *CharmHubCharm     `json:"charm,omitempty"`
	Bundle      *CharmHubBundle    `json:"bundle,omitempty"`
	Channels    map[string]Channel `json:"channel-map"`
	Tracks      []string           `json:"tracks"`
}

type CharmHubEntityFindResult struct {
	Results []FindResponse `json:"result"`
	Errors  ErrorResponse  `json:"errors"`
}

type FindResponse struct {
	Type      string   `json:"type"`
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Publisher string   `json:"publisher"`
	Summary   string   `json:"summary"`
	Version   string   `json:"version"`
	Series    []string `json:"series"`
	StoreURL  string   `json:"store-url"`
}

type Channel struct {
	ReleasedAt string `json:"released-at"`
	Track      string `json:"track"`
	Risk       string `json:"risk"`
	Revision   int    `json:"revision"`
	Size       int    `json:"size"`
	Version    string `json:"version"`
}

type CharmHubCharm struct {
	Config      map[string]CharmOption       `json:"config"`
	Relations   map[string]map[string]string `json:"relations"`
	Subordinate bool                         `json:"subordinate"`
	UsedBy      []string                     `json:"used-by"` // bundles which use the charm
}

type CharmHubBundle struct {
	Charms []BundleCharm `json:"charms,"`
}

type BundleCharm struct {
	Name     string `json:"name"`
	Revision int    `json:"revision"`
}

type ErrorResponse struct {
	Error CharmHubError `json:"error-list"`
}

type CharmHubError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
