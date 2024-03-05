// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
	"github.com/juju/juju/testing"
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
