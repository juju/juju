// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"github.com/juju/juju/core/status"
)

// ID is a provider-specific identifier associated with an
// instance (physical or virtual machine allocated in the provider).
type ID string

// Status represents the status for a provider instance.
type Status struct {
	Status  status.Status
	Message string
}

// UnknownID can be used to explicitly specify the instance ID does not matter.
const UnknownID ID = ""
