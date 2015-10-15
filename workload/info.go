// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"reflect"

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

// Info holds information about a workload that Juju needs. Iff the
// workload has not been registered with Juju then the Status and
// Details fields will be zero values.
//
// A registered workload is one which has been defined in Juju (e.g. in
// charm metadata) and subsequently was launched by Juju (e.g. in a
// unit hook context).
type Info struct {
	charm.Workload

	// Status is the Juju-level status of the workload.
	Status Status

	// Tags is the set of tags associated with the workload.
	Tags []string

	// Details is the information about the workload which the plugin provided.
	Details Details
}

// ID returns a uniqueID for a workload (relative to the unit/charm).
func (info Info) ID() string {
	return BuildID(info.Workload.Name, info.Details.ID)
}

// Validate checks the workload info to ensure it is correct.
func (info Info) Validate() error {
	if err := info.Workload.Validate(); err != nil {
		return errors.NewNotValid(err, "")
	}

	if err := info.Status.Validate(); err != nil {
		return errors.Trace(err)
	}

	if err := info.Details.Validate(); err != nil {
		return errors.Trace(err)
	}

	return nil
}

// IsTracked indicates whether the represented workload has
// is already being tracked by Juju.
func (info Info) IsTracked() bool {
	// An untracked workload will not have the Status and Details
	// fields set (they will be zero values). Thus a trackeded
	// workload can be identified by non-zero values in those fields.
	// We use that fact here.
	return !reflect.DeepEqual(info, Info{Workload: info.Workload})
}

// AsPayload converts the Info into a Payload.
func (info Info) AsPayload() Payload {
	tags := make([]string, len(info.Tags))
	copy(tags, info.Tags)
	return Payload{
		PayloadClass: charm.PayloadClass{
			Name: info.Name,
			Type: info.Type,
		},
		ID:     info.Details.ID,
		Status: info.Status.State,
		Tags:   tags,
	}
}
