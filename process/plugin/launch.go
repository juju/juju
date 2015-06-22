// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package plugin

import (
	"encoding/json"

	"github.com/juju/errors"
)

// ProcDetails represents information about a process launched by a plugin.
type ProcDetails struct {
	// ID is a unique string identifying the process to the plugin.
	ID string `json:"id"`
	// Status is the plugin-defined status of the process after launch.
	Status
}

func UnmarshalDetails(b []byte) (ProcDetails, error) {
	details := ProcDetails{}
	if err := json.Unmarshal(b, &details); err != nil {
		return details, errors.Annotate(err, "error parsing data for procdetails")
	}
	if err := details.validate(); err != nil {
		return details, errors.Annotate(err, "invalid procdetails")
	}
	return details, nil

}

// validate returns nil if this value is valid, and an error that satisfies
// IsValid if it is not.
func (p ProcDetails) validate() error {
	if p.ID == "" {
		e := errors.NewErr("ID cannot be empty")
		return validationErr{&e}
	}
	return p.Status.validate()
}
