// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import "github.com/juju/juju/internal/errors"

// ModelUpgradeBlocker describes an error that occurs when a model is blocked
// from being upgraded.
type ModelUpgradeBlocker struct {
	// Reason describes why the model cannot be upgraded because of this blocker.
	Reason string
}

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

	// AgentVersionNotSupported describes an error that occurs when an agent
	// version number is provided but the version is not supported.
	AgentVersionNotSupported = errors.ConstError("agent version not supported")

	// CannotUpgradeControllerModel describes an error that occurs when a model
	// upgrade is attempted for the model that hosts the current Juju controller.
	CannotUpgradeControllerModel = errors.ConstError("controller model cannot be upgraded")

	// DowngradeNotSupported describes an error that occurs when a downgrade of
	// an agent version is attempted, but the model does not support downgrades.
	DowngradeNotSupported = errors.ConstError("downgrade not supported")

	// LatestVersionDowngradeNotSupported describes an error that occurs when a
	// client attempts to set the latest agent version marker to a version that
	// is lower than the current agent version or current latest agent version.
	LatestVersionDowngradeNotSupported = errors.ConstError("latest version downgrade not supported")

	// MissingAgentBinaries describes an error that occurs when agent binaries
	// are missing for a given entity that runs agent binaries within the
	// model, eg units and machines. When agent binaries are missing, it
	// means that the model does not have a copy of the binaries.
	MissingAgentBinaries = errors.ConstError("missing agent binaries")
)

// Error returns an error message describing why a model upgrade is blocked.
// Implements the [error] interface.
func (e ModelUpgradeBlocker) Error() string {
	return "model upgrade blocked: " + e.Reason
}
