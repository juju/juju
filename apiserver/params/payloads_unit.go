// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// TrackPayloadParams contains payload information
// used to enable tracking by hook tool.
type TrackPayloadParams struct {
	// Class is the name of the payload class.
	Class string `json:"class"`

	// Type is the name of the payload type.
	Type string `json:"type"`

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string `json:"id"`

	// Status is the Juju-level status for the payload.
	Status string `json:"status"`

	// Labels are labels associated with the payload.
	Labels []string `json:"labels"`
}

// TrackPayloadParams are the collection of payloads
// to track from hook tool.
type TrackPayloadsParams struct {
	// Payloads is the list of Payloads to track
	Payloads []TrackPayloadParams `json:"payloads"`
}

// PayloadStatusParams contains payload information
// used to set payload status by hook tool.
type PayloadStatusParams struct {
	// Class is the name of the payload class.
	Class string `json:"class"`

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string `json:"id"`

	// Status is the Juju-level status for the payload.
	Status string `json:"status"`
}

// PayloadsStatusParams are the collection of payloads
// to set status from hook tool.
type PayloadsStatusParams struct {
	// Payloads is the list of Payloads to track
	Payloads []PayloadStatusParams `json:"payloads"`
}

// UntrackPayloadParams contains payload information
// used to stop tracking by hook tool.
type UntrackPayloadParams struct {
	// Class is the name of the payload class.
	Class string `json:"class"`

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string `json:"id"`
}

// UntrackPayloadsParams are the collection of payloads
// to stop tracking from hook tool.
type UntrackPayloadsParams struct {
	// Payloads is the list of Payloads to track
	Payloads []UntrackPayloadParams `json:"payloads"`
}
