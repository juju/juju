// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package process

import (
	"encoding/json"

	"github.com/juju/errors"
)

// Details represents information about a process launched by a plugin.
type Details struct {
	// ID is a unique string identifying the process to the plugin.
	ID string `json:"id"`
	// Status is the most recent plugin-defined status of the process.
	Status PluginStatus `json:"status"`
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
// IsNotValid if it is not.
func (p Details) Validate() error {
	if p.ID == "" {
		return errors.NotValidf("ID cannot be empty")
	}
	return p.Status.Validate()
}
