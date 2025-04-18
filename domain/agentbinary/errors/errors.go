// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/juju/internal/errors"
)

const (
	// AlreadyExists defines an error that indicates the agent binary with specified version,
	// architecture and SHA already exists in the database.
	AlreadyExists = errors.ConstError("agent binary already exists")

	// AgentBinaryImmutable defines an error that indicates the agent binary is immutable.
	// This error is returned when the agent binary with the specified version and architecture already
	// exists in the database but the SHA does not match.
	// This is used to prevent the agent binary from being modified after it has been created.
	AgentBinaryImmutable = errors.ConstError("agent binary is immutable")

	// NotFound defines an error that occurs when an agent binary is request and
	// it doesn't exist.
	NotFound = errors.ConstError("agent binary not found")

	// HashMismatch is returned when the hash of the agent binary does not match
	// the expected hash.
	HashMismatch = errors.ConstError("agent binary has mismatch")

	// ObjectNotFound defines an error that indicates the binary object
	// associated with the agent binary does not exist.
	ObjectNotFound = errors.ConstError("agent binary object not found")
)
