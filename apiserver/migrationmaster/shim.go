// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster

import (
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/migration"
	"github.com/juju/juju/state"
)

// newAPIForRegistration exists to provide the required signature for
// RegisterStandardFacade, converting st to backend.
func newAPIForRegistration(
	st *state.State,
	resources facade.Resources,
	authorizer facade.Authorizer,
) (*API, error) {
	return NewAPI(st, migration.PrecheckShim(st), resources, authorizer)
}
