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
	InstanceKey     string                 `json:"instance-key"`
	ID              string                 `json:"id"`
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
	ID          string                  `json:"id"`
	Channel     *string                 `json:"channel,omitempty"`
	Revision    *int                    `json:"revision,omitempty"`
	Platform    *RefreshRequestPlatform `json:"platform,omitempty"`
}

type RefreshResponses struct {
	Results   []RefreshResponse `json:"results"`
	ErrorList []APIError        `json:"error-list"`
}

type RefreshResponse struct {
	InstanceKey string `json:"instance-key"`
	ID          string `json:"id"`
	Name        string `json:"name"`
	// TODO (stickupkid): Swap this over to the new name if it ever happens.
	Entity Entity `json:"charm"`
	Result string `json:"result"`
	// TODO (stickupkid): Add Redirect-Channel and Effective-Channel.
}
