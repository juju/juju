// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lifeflag

import (
	"github.com/juju/juju/apiserver/facade"
)

// NewExternalFacade is for API registration.
func NewExternalFacade(ctx facade.Context) (*Facade, error) {
	return NewFacade(ctx.State(), ctx.Resources(), ctx.Auth())
}
