// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AgentVersionNotFound describes an error that occurs
	// when an agent version record is not present.
	AgentVersionNotFound = errors.ConstError("agent version not found")
)
