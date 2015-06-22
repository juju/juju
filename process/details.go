// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"encoding/json"

	"github.com/juju/errors"
)

// RawStatus represents the data returned from the Plugin.Status call.
type RawStatus struct {
	// Status represents the human-readable string returned by the plugin for
	// the process.
	Status string `json:"status"`
}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (s RawStatus) Validate() error {
	if s.Status == "" {
		e := errors.NewErr("Status cannot be empty")
		return validationErr{&e}
	}
	return nil
}

// Details represents information about a process launched by a plugin.
type Details struct {
	// ID is a unique string identifying the process to the plugin.
	ID string `json:"id"`
	// Status is the most recent plugin-defined status of the process.
	Status RawStatus `json:"status"`
}

// UnmarshalDetails de-serialized the provided data into a Details.
func UnmarshalDetails(b []byte) (Details, error) {
	var details Details
	if err := json.Unmarshal(b, &details); err != nil {
		return details, errors.Annotate(err, "error parsing data for procdetails")
	}
	if err := details.Validate(); err != nil {
		return details, errors.Annotate(err, "invalid procdetails")
	}
	return details, nil

}

// Validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p Details) Validate() error {
	if p.ID == "" {
		e := errors.NewErr("ID cannot be empty")
		return validationErr{&e}
	}
	return p.Status.Validate()
}
