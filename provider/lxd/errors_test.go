// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&ErrorSuite{})

func (s *ErrorSuite) TestIsUnauthorisedError(c *gc.C) {
	err := errors.New("not authorized")
	c.Assert(IsAuthorisationFailure(err), jc.IsTrue)
	c.Assert(IsAuthorisationFailure(errors.Cause(err)), jc.IsTrue)

	traced := errors.Trace(err)
	c.Assert(IsAuthorisationFailure(traced), jc.IsTrue)

	annotated := errors.Annotate(err, "testing is great")
	c.Assert(IsAuthorisationFailure(annotated), jc.IsTrue)
}

func (s *ErrorSuite) TestNotUnauthorisedError(c *gc.C) {
	err := errors.New("everything is fine")
	c.Assert(IsAuthorisationFailure(err), jc.IsFalse)

	c.Assert(IsAuthorisationFailure(nil), jc.IsFalse)
}
