// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	gc "gopkg.in/check.v1"

	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

type ErrorsSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (t *ErrorsSuite) TestNotAvailableErr(c *gc.C) {
	err := jujuc.NotAvailable("the thing")
	c.Assert(err, gc.ErrorMatches, "the thing is not available")
	c.Assert(jujuc.IsNotAvailable(err), jc.IsTrue)
}
