// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"reflect"

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

	// CharmID is the Juju ID for the process's charm.
	CharmID string

	// CharmID is the Juju ID for the process's unit.
	UnitID string

	// Details is the information about the process which the plugin provided.
	Details Details
}

// TODO(ericsnow) Eliminate NewInfoUnvalidated.

// NewInfoUnvalidated builds a new Info object with the provided
// values. The returned Info may be invalid if the given values cause
// that result. The Validate method can be used to check.
func NewInfoUnvalidated(name, procType string) *Info {
	return &Info{
		Process: charm.Process{
			Name: name,
			Type: procType,
		},
	}
}

// Validate checks the process info to ensure it is correct.
func (info Info) Validate() error {
	if err := info.Process.Validate(); err != nil {
		return errors.Trace(err)
	}

	if info.CharmID == "" {
		return errors.Errorf("missing CharmID")
	}

	if info.Details.ID != "" && info.UnitID == "" {
		return errors.Errorf("missing UnitID")
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
