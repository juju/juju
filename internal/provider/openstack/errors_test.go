// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack

import (
	gooseerrors "github.com/go-goose/goose/v5/errors"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
)

type ErrorSuite struct {
	testing.BaseSuite
}

var _ = tc.Suite(&ErrorSuite{})

func (s *ErrorSuite) TestIsUnauthorisedError(c *tc.C) {
	e := gooseerrors.NewUnauthorisedf(nil, "", "not on")
	c.Assert(IsAuthorisationFailure(e), tc.IsTrue)
	c.Assert(IsAuthorisationFailure(errors.Cause(e)), tc.IsTrue)

	traced := errors.Trace(e)
	c.Assert(IsAuthorisationFailure(traced), tc.IsTrue)

	annotated := errors.Annotatef(e, "more and more")
	c.Assert(IsAuthorisationFailure(annotated), tc.IsTrue)
}

func (s *ErrorSuite) TestIsNotUnauthorisedErro(c *tc.C) {
	e := errors.New("fluffy")
	c.Assert(IsAuthorisationFailure(e), tc.IsFalse)

	c.Assert(IsAuthorisationFailure(nil), tc.IsFalse)
}
