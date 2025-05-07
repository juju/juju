// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	gooseerrors "github.com/go-goose/goose/v5/errors"
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
	e := gooseerrors.NewUnauthorisedf(nil, "", "not on")
	c.Assert(IsAuthorisationFailure(e), jc.IsTrue)
	c.Assert(IsAuthorisationFailure(errors.Cause(e)), jc.IsTrue)

	traced := errors.Trace(e)
	c.Assert(IsAuthorisationFailure(traced), jc.IsTrue)

	annotated := errors.Annotatef(e, "more and more")
	c.Assert(IsAuthorisationFailure(annotated), jc.IsTrue)
}

func (s *ErrorSuite) TestIsNotUnauthorisedErro(c *tc.C) {
	e := errors.New("fluffy")
	c.Assert(IsAuthorisationFailure(e), jc.IsFalse)

	c.Assert(IsAuthorisationFailure(nil), jc.IsFalse)
}
