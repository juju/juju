// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The resource package provides the functionality of the "resources"
// feature in Juju.
package resource

import (
	"time"

	"github.com/juju/errors"
)

// Resource defines a single resource within Juju state.
type Resource struct {
	Info

	// Username is the ID of the user that added the revision
	// to the model (whether implicitly or explicitly).
	Username string

	// Timestamp indicates when the resource was added to the model.
	Timestamp time.Time
}

// Validate ensures that the spec is valid.
func (res Resource) Validate() error {
	if err := res.Info.Validate(); err != nil {
		return errors.Annotate(err, "bad info")
	}

	if len(res.Username) == 0 {
		return errors.NewNotValid(nil, "missing username")
	}

	if res.Timestamp.IsZero() {
		return errors.NewNotValid(nil, "missing timestamp")
	}

	return nil
}
