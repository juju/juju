// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"gopkg.in/juju/charm.v5"
)

// Info holds information about a process that Juju needs. Iff the
// process has not been registered with Juju then the Status and
// Details fields will be zero values.
//
// A registered process is one which has been defined in Juju (e.g. in
// charm metadata) and subsequently was launched by Juju (e.g. in a
// unit hook context).
type Info struct {
	charm.Process

	// Status is the Juju-level status of the process.
	Status Status

	// Details is the information about the process which the plugin provided.
	Details Details
}

// ID composes a unique ID for the process (relative to the unit/charm).
func (info Info) ID() string {
	id := info.Process.Name
	if info.Details.ID != "" {
		id = fmt.Sprintf("%s/%s", id, info.Details.ID)
	}
	return id
}

// ParseID extracts the process name and details ID from the provided string.
func ParseID(id string) (string, string) {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}

// Validate checks the process info to ensure it is correct.
func (info Info) Validate() error {
	if err := info.Process.Validate(); err != nil {
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

// IsRegistered indicates whether the represented process has already
// been registered with Juju.
func (info Info) IsRegistered() bool {
	// An unregistered process will not have the Status and Details
	// fields set (they will be zero values). Thus a registered
	// process can be identified by non-zero values in those fields.
	// We use that fact here.
	return !reflect.DeepEqual(info, Info{Process: info.Process})
}
