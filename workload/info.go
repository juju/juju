// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"reflect"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
)

// Info holds information about a workload that Juju needs. Iff the
// workload has not been registered with Juju then the Status and
// Details fields will be zero values.
//
// A registered workload is one which has been defined in Juju (e.g. in
// charm metadata) and subsequently was launched by Juju (e.g. in a
// unit hook context).
type Info struct {
	charm.PayloadClass

	// Status is the Juju-level status of the workload.
	Status Status

	// Labels is the set of labels associated with the workload.
	Labels []string

	// Details is the information about the workload which the plugin provided.
	Details Details
}

// ID returns a uniqueID for a workload (relative to the unit/charm).
func (info Info) ID() string {
	return BuildID(info.PayloadClass.Name, info.Details.ID)
}

// Validate checks the workload info to ensure it is correct.
func (info Info) Validate() error {
	if err := info.PayloadClass.Validate(); err != nil {
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
	return !reflect.DeepEqual(info, Info{PayloadClass: info.PayloadClass})
}

// AsPayload converts the Info into a Payload.
func (info Info) AsPayload() Payload {
	labels := make([]string, len(info.Labels))
	copy(labels, info.Labels)
	return Payload{
		PayloadClass: charm.PayloadClass{
			Name: info.Name,
			Type: info.Type,
		},
		ID:     info.Details.ID,
		Status: info.Status.State,
		Labels: labels,
	}
}
