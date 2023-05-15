// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication

import (
	"github.com/juju/errors"
)

const (
	// ErrorEntityMissingPermission is an error that acts as an anchor for all
	// consumers of authorization information in the API. As authorizers may
	// return different authorization errors based on the entity context. This
	// error exists to wrap all of these difference so one value can be used to
	// determine if an authorization problem has occurred.
	ErrorEntityMissingPermission = errors.ConstError("entity missing permission")
)
