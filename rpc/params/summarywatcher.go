// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SummaryWatcherID holds the id of a model summary watcher.
type SummaryWatcherID struct {
	WatcherID string `json:"watcher-id"`
}

// SummaryWatcherNextResults holds deltas returned from calling AllWatcher.Next().
type SummaryWatcherNextResults struct {
	Models []ModelAbstract `json:"models"`
}

// ModelAbstract represents a summary of a model.
// Unfortunately we already have a ModelSummary in the params package.
type ModelAbstract struct {
	UUID    string `json:"uuid"`
	Removed bool   `json:"removed,omitzero"`

	Controller string   `json:"controller,omitempty"`
	Name       string   `json:"name,omitempty"`
	Admins     []string `json:"admins,omitempty"`

	Cloud      string `json:"cloud,omitempty"`
	Region     string `json:"region,omitempty"`
	Credential string `json:"credential,omitempty"`

	Size ModelSummarySize `json:"size,omitempty"`

	Status   string                `json:"status,omitempty"`
	Messages []ModelSummaryMessage `json:"messages,omitempty"`

	Annotations map[string]string `json:"annotations,omitempty"`
}

// ModelSummarySize represents the number of various entities in the model.
type ModelSummarySize struct {
	Machines     int `json:"machines,omitzero"`
	Containers   int `json:"containers,omitzero"`
	Applications int `json:"applications,omitzero"`
	Units        int `json:"units,omitzero"`
	Relations    int `json:"relations,omitzero"`
}

// ModelSummaryMessage represents a non-green status from an agent.
type ModelSummaryMessage struct {
	Agent   string `json:"agent"`
	Message string `json:"message"`
}
