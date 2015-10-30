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

// AsWorkload converts the Payload into an Info.
func (p Payload) AsWorkload() Info {
	labels := make([]string, len(p.Labels))
	copy(labels, p.Labels)
	return Info{
		PayloadClass: charm.PayloadClass{
			Name: p.Name,
			Type: p.Type,
		},
		Status: Status{
			State: p.Status,
		},
		Labels: labels,
		Details: Details{
			ID: p.ID,
		},
	}
}

// FullPayloadInfo completely describes a charm payload, including
// some information that may be implied from the minimal Payload data.
type FullPayloadInfo struct {
	Payload

	// Machine identifies the Juju machine associated with the payload.
	Machine string
}
