// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"github.com/juju/errors"
)

// Status represents the data returned from the Plugin.Status call.
type Status struct {
	// Label represents the human-readable label returned by the plugin
	// that represents the status of the workload process.
	Label string `json:"label"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s Status) Validate() error {
	if s.Label == "" {
		return errors.NotValidf("Label cannot be empty")
	}
	return nil
}
