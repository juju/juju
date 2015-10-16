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

	// Tags are tags associated with the payload.
	Tags []string

	// Unit identifies the Juju unit associated with the payload.
	Unit string

	// Machine identifies the Juju machine associated with the payload.
	Machine string
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

	if p.Unit == "" {
		return errors.NotValidf("missing Unit")
	}

	if p.Machine == "" {
		return errors.NotValidf("missing Machine")
	}

	return nil
}

// AsWorkload converts the Payload into an Info.
func (p Payload) AsWorkload() Info {
	tags := make([]string, len(p.Tags))
	copy(tags, p.Tags)
	return Info{
		Workload: charm.Workload{
			Name: p.Name,
			Type: p.Type,
		},
		Status: Status{
			State: p.Status,
		},
		Tags: tags,
		Details: Details{
			ID: p.ID,
		},
	}
}
