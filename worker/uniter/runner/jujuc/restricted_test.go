// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type restrictedContext struct {
	*jujuc.RestrictedContext
}

// UnitName completes the jujuc.Context implementation, which the
// RestrictedContext leaves out.
func (*restrictedContext) UnitName() string { return "restricted" }

var _ jujuc.Context = (*restrictedContext)(nil)
