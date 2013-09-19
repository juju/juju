// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/state/api/params"
)

var ErrUnauthorized = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}

func NotFoundError(prefixMessage string) *params.Error {
	return &params.Error{
		Message: prefixMessage + " not found",
		Code:    params.CodeNotFound,
	}
}
