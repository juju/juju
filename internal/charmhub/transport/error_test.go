// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"encoding/json"

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
	err := errors.Error()
	c.Assert(err, gc.DeepEquals, "")
}

func (ErrorSuite) TestNoErrorsWithEmptySlice(c *gc.C) {
	errors := make(APIErrors, 0)
	err := errors.Error()
	c.Assert(err, gc.DeepEquals, "")
}

func (ErrorSuite) TestWithOneError(c *gc.C) {
	errors := APIErrors{{
		Message: "one",
	}}
	err := errors.Error()
	c.Assert(err, gc.DeepEquals, `one`)
}

func (ErrorSuite) TestWithMultipleErrors(c *gc.C) {
	errors := APIErrors{
		{Message: "one"},
		{Message: "two"},
	}
	err := errors.Error()
	c.Assert(err, gc.DeepEquals, `one
two`)
}

func (ErrorSuite) TestExtras(c *gc.C) {
	expected := APIError{
		Extra: APIErrorExtra{
			DefaultBases: []Base{
				{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
			},
		},
	}
	bytes, err := json.Marshal(expected)
	c.Assert(err, jc.ErrorIsNil)

	var result APIError
	err = json.Unmarshal(bytes, &result)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(result, gc.DeepEquals, expected)
}
