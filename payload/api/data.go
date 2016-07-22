// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Move this file to the top-level "payload" package?

// EnvListArgs are the arguments for the env-based List endpoint.
type EnvListArgs struct {
	// Patterns is the list of patterns against which to filter.
	Patterns []string `json:"patterns"`
}

type EnvListResults struct {
	// Results is the list of payload results.
	Results []Payload `json:"results"`
}

// Payload contains full information about a payload.
type Payload struct {
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

	// Unit identifies the unit tag associated with the payload.
	Unit string `json:"unit"`
	// Machine identifies the machine tag associated with the payload.
	Machine string `json:"machine"`
}
