// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuc_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/worker/uniter/runner/jujuc"
)

type ErrorsSuite struct {
	testing.BaseSuite
}

func TestErrorsSuite(t *stdtesting.T) {
	tc.Run(t, &ErrorsSuite{})
}

func (t *ErrorsSuite) TestNotAvailableErr(c *tc.C) {
	err := jujuc.NotAvailable("the thing")
	c.Assert(err, tc.ErrorMatches, "the thing is not available")
	c.Assert(jujuc.IsNotAvailable(err), tc.IsTrue)
}
