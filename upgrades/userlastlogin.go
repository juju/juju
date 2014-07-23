// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades

import (
	"github.com/juju/juju/state"
)

// The LastConnection field on the user document should be renamed to
// LastLogin.
func migrateLastConnectionToLastLogin(context Context) error {
	return state.MigrateUserLastConnectionToLastLogin(context.State())
}
