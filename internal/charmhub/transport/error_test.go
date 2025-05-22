// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package transport

import (
	"encoding/json"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type ErrorSuite struct {
	testhelpers.IsolationSuite
}

func TestErrorSuite(t *testing.T) {
	tc.Run(t, &ErrorSuite{})
}

func (s *ErrorSuite) TestNoErrors(c *tc.C) {
	var errors APIErrors
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, "")
}

func (s *ErrorSuite) TestNoErrorsWithEmptySlice(c *tc.C) {
	errors := make(APIErrors, 0)
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, "")
}

func (s *ErrorSuite) TestWithOneError(c *tc.C) {
	errors := APIErrors{{
		Message: "one",
	}}
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, `one`)
}

func (s *ErrorSuite) TestWithMultipleErrors(c *tc.C) {
	errors := APIErrors{
		{Message: "one"},
		{Message: "two"},
	}
	err := errors.Error()
	c.Assert(err, tc.DeepEquals, `one
two`)
}

func (s *ErrorSuite) TestExtras(c *tc.C) {
	expected := APIError{
		Extra: APIErrorExtra{
			DefaultBases: []Base{
				{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
			},
		},
	}
	bytes, err := json.Marshal(expected)
	c.Assert(err, tc.ErrorIsNil)

	var result APIError
	err = json.Unmarshal(bytes, &result)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(result, tc.DeepEquals, expected)
}
