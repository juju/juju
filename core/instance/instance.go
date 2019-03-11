// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instance

import (
	"github.com/juju/juju/core/status"
)

// Id is a provider-specific identifier associated with an
// instance (physical or virtual machine allocated in the provider).
type Id string

// Status represents the status for a provider instance.
type Status struct {
	Status  status.Status
	Message string
}

// UnknownId can be used to explicitly specify the instance Id when it does not matter.
const UnknownId Id = ""
