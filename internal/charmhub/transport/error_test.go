// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"encoding/json"

	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
)

type ErrorSuite struct {
	testing.IsolationSuite
}

var _ = tc.Suite(&ErrorSuite{})

func (ErrorSuite) TestNoErrors(c *tc.C) {
	var errors APIErrors
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, "")
}

func (ErrorSuite) TestNoErrorsWithEmptySlice(c *tc.C) {
	errors := make(APIErrors, 0)
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, "")
}

func (ErrorSuite) TestWithOneError(c *tc.C) {
	errors := APIErrors{{
		Message: "one",
	}}
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, `one`)
}

func (ErrorSuite) TestWithMultipleErrors(c *tc.C) {
	errors := APIErrors{
		{Message: "one"},
		{Message: "two"},
	}
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, `one
two`)
}

func (ErrorSuite) TestExtras(c *tc.C) {
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

	c.Assert(result, tc.DeepEquals, expected)
}
