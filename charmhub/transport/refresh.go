// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import "time"

// RefreshRequest defines a typed request for making refresh queries, containing
// both a series of context and actions, this powerful setup should allow for
// making batch queries where possible.
type RefreshRequest struct {
	// Context can be empty (for install and download for example), but has to
	// be always present and hence the no omitempty.
	Context []RefreshRequestContext `json:"context"`
	Actions []RefreshRequestAction  `json:"actions"`
}

// RefreshRequestContext can request a given context for making multiple
// requests to one given entity.
type RefreshRequestContext struct {
	InstanceKey string `json:"instance-key"`
	ID          string `json:"id"`

	Revision        int        `json:"revision"`
	Platform        Platform   `json:"platform,omitempty"`
	TrackingChannel string     `json:"tracking-channel,omitempty"`
	RefreshedDate   *time.Time `json:"refresh-date,omitempty"`
}

// RefreshRequestAction defines a action to perform against the Refresh API.
type RefreshRequestAction struct {
	// Action can be install, download or refresh.
	Action string `json:"action"`
	// InstanceKey should be unique for every action, as results may not be
	// ordered in the same way, so it is expected to use this to ensure
	// completeness and ordering.
	InstanceKey string    `json:"instance-key"`
	ID          *string   `json:"id,omitempty"`
	Name        *string   `json:"name,omitempty"`
	Channel     *string   `json:"channel,omitempty"`
	Revision    *int      `json:"revision,omitempty"`
	Platform    *Platform `json:"platform,omitempty"`
}

// RefreshResponses holds a series of typed RefreshResponse or a series of
// errors if the query performed failed for some reason.
type RefreshResponses struct {
	Results   []RefreshResponse `json:"results,omitempty"`
	ErrorList APIErrors         `json:"error-list,omitempty"`
}

// RefreshResponse defines a typed response for the refresh query.
type RefreshResponse struct {
	Entity           RefreshEntity `json:"charm"` // TODO: Pick up new naming of this.
	EffectiveChannel string        `json:"effective-channel"`
	Error            *APIError     `json:"error,omitempty"`
	ID               string        `json:"id"`
	InstanceKey      string        `json:"instance-key"`
	Name             string        `json:"name"`
	Result           string        `json:"result"`

	// Officially the released-at is ISO8601, but go's version of time.Time is
	// both RFC3339 and ISO8601 (the latter makes the T optional).
	ReleasedAt time.Time `json:"released-at"`
}

// RefreshEntity is a typed refresh entity.
// This can either be a charm or a bundle, the type of the refresh entity
// doesn't actually matter in this circumstance.
type RefreshEntity struct {
	Type      Type               `json:"type"`
	Download  Download           `json:"download"`
	ID        string             `json:"id"`
	License   string             `json:"license"`
	Name      string             `json:"name"`
	Publisher map[string]string  `json:"publisher,omitempty"`
	Resources []ResourceRevision `json:"resources"`
	Revision  int                `json:"revision"`
	Summary   string             `json:"summary"`
	Version   string             `json:"version"`
	CreatedAt time.Time          `json:"created-at"`
}
