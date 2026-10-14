// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// SSHServerPortNotFound describes an error that occurs when the controller
	// SSH server port has not been set.
	SSHServerPortNotFound = errors.ConstError("ssh server port not found")
)
