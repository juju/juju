// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"github.com/juju/errors"
)

// ProcStatus represents the data returned from the Status call.
type ProcStatus struct {
	// Status represents the human-readable string returned by the plugin for
	// the process.
	Status string `json:"status"`
}

// validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p ProcStatus) validate() error {
	if p.Status == "" {
		e := errors.NewErr("Status cannot be empty")
		return validationErr{&e}
	}
	return nil
}
