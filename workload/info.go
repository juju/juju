// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"reflect"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
)

// Payload holds information about a charm payload.
type Payload struct {
	charm.PayloadClass

	// ID is a unique string identifying the payload to
	// the underlying technology.
	ID string

	// Status is the Juju-level status of the workload.
	Status string

	// Tags are tags associated with the payload.
	Tags []string

	// Unit identifies the Juju unit associated with the payload.
	Unit string

	// Machine identifies the Juju machine associated with the payload.
	Machine string
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

// BuildID composes an ID from a class and id
func BuildID(class, id string) string {
	if id == "" {
		// TODO(natefinch) remove this special case when we can be sure the ID
		// is never empty (and fix the tests).
		return class
	}
	return class + "/" + id
}

// ParseID extracts the workload name and details ID from the provided string.
// The format is expected to be name/pluginID. If no separator is found, the
// whole string is assumed to be the name.
func ParseID(id string) (name, pluginID string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
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
