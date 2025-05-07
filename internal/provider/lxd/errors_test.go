// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ErrorSuite{})

func (s *ErrorSuite) TestIsUnauthorisedError(c *tc.C) {
	err := errors.New("not authorized")
	c.Assert(IsAuthorisationFailure(err), jc.IsTrue)
	c.Assert(IsAuthorisationFailure(errors.Cause(err)), jc.IsTrue)

	traced := errors.Trace(err)
	c.Assert(IsAuthorisationFailure(traced), jc.IsTrue)

	annotated := errors.Annotate(err, "testing is great")
	c.Assert(IsAuthorisationFailure(annotated), jc.IsTrue)
}

func (s *ErrorSuite) TestNotUnauthorisedError(c *tc.C) {
	err := errors.New("everything is fine")
	c.Assert(IsAuthorisationFailure(err), jc.IsFalse)

	c.Assert(IsAuthorisationFailure(nil), jc.IsFalse)
}
