// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import "time"

type RefreshRequest struct {
	// Context can be empty (for install and download for example), but has to
	// be always present and hence the no omitempty.
	Context []RefreshRequestContext `json:"context"`
	Actions []RefreshRequestAction  `json:"actions"`
}

type RefreshRequestContext struct {
	InstanceKey string `json:"instance-key"`
	ID          string `json:"id"`

	Revision        int                    `json:"revision"`
	Platform        RefreshRequestPlatform `json:"platform"`
	TrackingChannel string                 `json:"tracking-channel"`
	RefreshedDate   *time.Time             `json:"refresh-date,omitempty"`
}

type RefreshRequestPlatform struct {
	OS           string `json:"os"`
	Series       string `json:"series"`
	Architecture string `json:"architecture"`
}

type RefreshRequestAction struct {
	Action      string                  `json:"action"`
	InstanceKey string                  `json:"instance-key"`
	ID          *string                 `json:"id"`
	Name        *string                 `json:"name"`
	Channel     *string                 `json:"channel,omitempty"`
	Revision    *int                    `json:"revision,omitempty"`
	Platform    *RefreshRequestPlatform `json:"platform,omitempty"`
}

type RefreshResponses struct {
	Results   []RefreshResponse `json:"results"`
	ErrorList []APIError        `json:"error-list"`
}

type RefreshResponse struct {
	// TODO (stickupkid): Swap this over to the new name if it ever happens.
	Entity           RefreshEntity `json:"charm"`
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

type RefreshEntity struct {
	CreatedAt string            `json:"created-at"`
	Download  Download          `json:"download"`
	ID        string            `json:"id"`
	License   string            `json:"license"`
	Name      string            `json:"name"`
	Publisher map[string]string `json:"publisher,omitempty"`
	Summary   string            `json:"summary"`
	Version   string            `json:"version"`
}
