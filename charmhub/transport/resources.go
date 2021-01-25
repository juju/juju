// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

type ResourcesResponse struct {
	Revisions []ResourceRevision `json:"revisions"`
}

type ResourceRevision struct {
	Download    Download `json:"download"`
	Description string   `json:"description"`
	Name        string   `json:"name"`
	Filename    string   `json:"filename"`
	Revision    int      `json:"revision"`
	Type        string   `json:"type"`
}
