// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"

	"launchpad.net/juju-core/state/api/params"
)

var ErrUnauthorized = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}

func NotFoundError(prefixMessage string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("%s not found", prefixMessage),
		Code:    params.CodeNotFound,
	}
}

func NotProvisionedError(machineId string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("machine %s is not provisioned", machineId),
		Code:    params.CodeNotProvisioned,
	}
}

func NotAssignedError(unitName string) *params.Error {
	return &params.Error{
		Message: fmt.Sprintf("unit %q is not assigned to a machine", unitName),
		Code:    params.CodeNotAssigned,
	}
}

func ServerError(message string) *params.Error {
	return &params.Error{
		Message: message,
		Code:    "",
	}
}
