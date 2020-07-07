// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type InfoResponse struct {
	Type           string       `json:"type"`
	ID             string       `json:"id"`
	Name           string       `json:"name"`
	Entity         Entity       `json:"charm"`
	ChannelMap     []ChannelMap `json:"channel-map"`
	DefaultRelease ChannelMap   `json:"default-release,omitempty"`
}
