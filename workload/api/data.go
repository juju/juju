// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

// TODO(ericsnow) Move this file to the top-level "payload" package?

// TODO(ericsnow) Eliminate the params import if possible.

import (
	"github.com/juju/errors"
	"github.com/juju/names"

	"github.com/juju/juju/apiserver/params"
)

// BulkFailure indicates that at least one arg failed.
var BulkFailure = errors.Errorf("at least one bulk arg has an error")

// EnvListArgs are the arguments for the env-based List endpoint.
type EnvListArgs struct {
	// Patterns is the list of patterns against which to filter.
	Patterns []string
}

type EnvListResults struct {
	// Results is the list of payload results.
	Results []Payload
	// Error is the error (if any) for the call as a whole.
	Error *params.Error
}

// Payload contains full information about a payload.
type Payload struct {
	// Class is the name of the payload class.
	Class string
	// Type is the name of the payload type.
	Type string

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string
	// Status is the Juju-level status for the payload.
	Status string
	// Tags are tags associated with the payload.
	Tags []string

	// Unit identifies the unit associated with the payload.
	Unit names.UnitTag
	// Machine identifies the machine associated with the payload.
	Machine names.MachineTag
}
