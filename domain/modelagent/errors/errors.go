// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

const (
	// AgentStreamNotValid describes an error that occurs when an agent stream
	// supplied is not considered valid for the operation.
	AgentStreamNotValid = errors.ConstError("agent stream not valid")

	// AgentVersionNotFound describes an error that occurs
	// when an agent version record is not present.
	AgentVersionNotFound = errors.ConstError("agent version not found")

	// AgentVersionNotSet describes an error that occurs when a machine
	// does not have its agent version set.
	AgentVersionNotSet = errors.ConstError("agent version not set")

	// MissingAgentBinaries describes an error that occurs when agent binaries
	// are missing for a given entity that runs agent binaries within the
	// model, eg units and machines. When agent binaries are missing, it
	// means that the model does not have a copy of the binaries.
	MissingAgentBinaries = errors.ConstError("missing agent binaries")
)
