// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationminion

import (
	"github.com/juju/juju/apiserver/facade"
)

// NewFacade provides the signature required for facade registration.
func NewFacade(ctx facade.Context) (*API, error) {
	return NewAPI(ctx.State(), ctx.Resources(), ctx.Auth())
}
