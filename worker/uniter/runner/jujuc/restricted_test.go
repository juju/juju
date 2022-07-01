// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/loggo"

	"github.com/juju/juju/v2/worker/uniter/runner/jujuc"
)

type restrictedContext struct {
	*jujuc.RestrictedContext
}

// UnitName completes the hooks.Context implementation, which the
// RestrictedContext leaves out.
func (*restrictedContext) UnitName() string { return "restricted" }

func (r *restrictedContext) GetLogger(m string) loggo.Logger {
	return loggo.GetLogger(m)
}

var _ jujuc.Context = (*restrictedContext)(nil)
