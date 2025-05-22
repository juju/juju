// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/internal/charmhub/transport"
)

type ErrorsSuite struct {
	baseSuite
}

func TestErrorsSuite(t *testing.T) {
	tc.Run(t, &ErrorsSuite{})
}

func (s *ErrorsSuite) TestHandleBasicAPIErrors(c *tc.C) {
	var list transport.APIErrors
	err := handleBasicAPIErrors(c.Context(), list, s.logger)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsNotFound(c *tc.C) {
	list := transport.APIErrors{{Code: transport.ErrorCodeNotFound, Message: "foo"}}
	err := handleBasicAPIErrors(c.Context(), list, s.logger)
	c.Assert(err, tc.ErrorMatches, `charm or bundle not found`)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsOther(c *tc.C) {
	list := transport.APIErrors{{Code: "other", Message: "foo"}}
	err := handleBasicAPIErrors(c.Context(), list, s.logger)
	c.Assert(err, tc.ErrorMatches, `foo`)
}
