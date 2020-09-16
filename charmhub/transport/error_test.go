// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type ErrorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorSuite{})

func (ErrorSuite) TestNoErrors(c *gc.C) {
	var errors APIErrors
	err := errors.Combine()
	c.Assert(err, jc.ErrorIsNil)
}

func (ErrorSuite) TestNoErrorsWithEmptySlice(c *gc.C) {
	errors := make(APIErrors, 0)
	err := errors.Combine()
	c.Assert(err, jc.ErrorIsNil)
}

func (ErrorSuite) TestWithOneError(c *gc.C) {
	errors := APIErrors{{
		Message: "one",
	}}
	err := errors.Combine()
	c.Assert(err, gc.ErrorMatches, `one`)
}

func (ErrorSuite) TestWithMultipleErrors(c *gc.C) {
	errors := APIErrors{
		{Message: "one"},
		{Message: "two"},
	}
	err := errors.Combine()
	c.Assert(err, gc.ErrorMatches, `one
two`)
}
