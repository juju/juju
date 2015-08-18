// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package workload

import (
	"fmt"
	"reflect"
	"strings"

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
	charm.Workload

	// Status is the Juju-level status of the workload.
	Status Status

	// Details is the information about the workload which the plugin provided.
	Details Details
}

// ID composes a unique ID for the workload (relative to the unit/charm).
func (info Info) ID() string {
	if info.Details.ID != "" {
		return fmt.Sprintf("%s/%s", info.Process.Name, info.Details.ID)
	}
	return info.Process.Name
}

// ParseID extracts the workload name and details ID from the provided string.
// The format is expected to be name/id. If no separator is found, the whole
// string is assumed to be the name.
func ParseID(s string) (name, ID string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return s, ""
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
