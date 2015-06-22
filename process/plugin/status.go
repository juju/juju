// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"github.com/juju/errors"
)

// Status represents the data returned from the Plugin.Status call.
type Status struct {
	// Status represents the human-readable string returned by the plugin for
	// the process.
	Status string `json:"status"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s Status) Validate() error {
	if s.Status == "" {
		e := errors.NewErr("Status cannot be empty")
		return validationErr{&e}
	}
	return nil
}
