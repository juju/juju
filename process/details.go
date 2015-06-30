// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"encoding/json"

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

// Details represents information about a process launched by a plugin.
type Details struct {
	// ID is a unique string identifying the process to the plugin.
	ID string `json:"id"`
	// Status is the most recent plugin-defined status of the process.
	Status Status `json:"status"`
}

// UnmarshalDetails de-serialized the provided data into a Details.
func UnmarshalDetails(b []byte) (Details, error) {
	var details Details
	if err := json.Unmarshal(b, &details); err != nil {
		return details, errors.Annotate(err, "error parsing data for workload process details")
	}
	if err := details.Validate(); err != nil {
		return details, errors.Annotate(err, "invalid workload process details")
	}
	return details, nil

}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p Details) Validate() error {
	if p.ID == "" {
		return errors.NotValidf("ID cannot be empty")
	}
	return p.Status.Validate()
}
