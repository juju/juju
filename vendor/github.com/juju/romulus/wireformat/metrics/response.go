// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metrics

// UserStatusResponse represents the response from the metrics collector that
// reports user meter statuses.
type UserStatusResponse struct {
	UUID          string        `json:"uuid"`
	UserResponses UserResponses `json:"user-responses"`
}

// UserResponses is a set of meter status and batch acknowledgement responses.
// Keyed off username.
type UserResponses map[string]UserResponse

// UserResponse is the response to a single user's metric batches.
type UserResponse struct {
	Status              MeterStatus `json:"status"`
	AcknowledgedBatches []string    `json:"acks,omitempty"`
}

// MeterStatus represents the meter status information.
type MeterStatus struct {
	Status string `json:"status"`
	Info   string `json:"info"`
}
