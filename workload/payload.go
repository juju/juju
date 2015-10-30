// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
)

// Payload holds information about a charm payload.
type Payload struct {
	charm.PayloadClass

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string

	// TODO(ericsnow) Use the workload.Status type instead of a string?

	// Status is the Juju-level status of the workload.
	Status string

	// Labels are labels associated with the payload.
	Labels []string

	// Unit identifies the Juju unit associated with the payload.
	Unit string
}

// FullID composes a unique ID for the payload (relative to the unit/charm).
func (p Payload) FullID() string {
	return BuildID(p.PayloadClass.Name, p.ID)
}

// Validate checks the payload info to ensure it is correct.
func (p Payload) Validate() error {
	if err := p.PayloadClass.Validate(); err != nil {
		return errors.NewNotValid(err, "")
	}

	if p.ID == "" {
		return errors.NotValidf("missing ID")
	}

	if err := ValidateState(p.Status); err != nil {
		return errors.Trace(err)
	}

	// TODO(ericsnow) Do not require Unit to be set?
	if p.Unit == "" {
		return errors.NotValidf("missing Unit")
	}

	return nil
}

// FullPayloadInfo completely describes a charm payload, including
// some information that may be implied from the minimal Payload data.
type FullPayloadInfo struct {
	Payload

	// Machine identifies the Juju machine associated with the payload.
	Machine string
}

// Result is a struct that ties an error to a workload ID.
type Result struct {
	// ID is the ID of the workload that this result applies to.
	ID string
	// Payload holds the info about the workload, if available.
	Payload *FullPayloadInfo
	// NotFound indicates that the workload was not found in Juju.
	NotFound bool
	// Error is the error associated with this result (if any).
	Error error
}
