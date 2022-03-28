// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// NewFacade wraps New to express the supplied *state.State as a Backend.
func NewFacade(ctx facade.Context) (*Facade, error) {
	facade, err := New(ctx.State(), ctx.Resources(), ctx.Auth())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return facade, nil
}
