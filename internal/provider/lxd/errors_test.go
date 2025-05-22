// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxd

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
}

func TestErrorSuite(t *stdtesting.T) {
	tc.Run(t, &ErrorSuite{})
}

func (s *ErrorSuite) TestIsUnauthorisedError(c *tc.C) {
	err := errors.New("not authorized")
	c.Assert(IsAuthorisationFailure(err), tc.IsTrue)
	c.Assert(IsAuthorisationFailure(errors.Cause(err)), tc.IsTrue)

	traced := errors.Trace(err)
	c.Assert(IsAuthorisationFailure(traced), tc.IsTrue)

	annotated := errors.Annotate(err, "testing is great")
	c.Assert(IsAuthorisationFailure(annotated), tc.IsTrue)
}

func (s *ErrorSuite) TestNotUnauthorisedError(c *tc.C) {
	err := errors.New("everything is fine")
	c.Assert(IsAuthorisationFailure(err), tc.IsFalse)

	c.Assert(IsAuthorisationFailure(nil), tc.IsFalse)
}
