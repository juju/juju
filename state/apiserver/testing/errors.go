// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"launchpad.net/juju-core/state/api/params"
)

var UnauthorizedError *params.Error = &params.Error{
	Message: "permission denied",
	Code:    params.CodeUnauthorized,
}
