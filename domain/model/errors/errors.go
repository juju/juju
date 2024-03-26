// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package errors

import (
	"github.com/juju/errors"
)

const (
	// AgentVersionNotSupported describes an error that occurs when then agent
	// version chosen for model is not supported with respect to the currently
	// running controller.
	AgentVersionNotSupported = errors.ConstError("agent version not supported")

	// AlreadyExists describes an error that occurs when a model already exists.
	AlreadyExists = errors.ConstError("model already exists")

	// AlreadyFinalised describes an error that occurs when an attempt is made
	// to finalise a model that has already been finalised.
	AlreadyFinalised = errors.ConstError("model already finalised")

	// NotFound describes an error that occurs when the model being operated on
	// does not exist.
	NotFound = errors.ConstError("model not found")
)
