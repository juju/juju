// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/charmhub/transport"
)

type ErrorsSuite struct {
	baseSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (s *ErrorsSuite) TestHandleBasicAPIErrors(c *gc.C) {
	var list transport.APIErrors
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsNotFound(c *gc.C) {
	list := transport.APIErrors{{Code: transport.ErrorCodeNotFound, Message: "foo"}}
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, gc.ErrorMatches, `charm or bundle not found`)
}

func (s *ErrorsSuite) TestHandleBasicAPIErrorsOther(c *gc.C) {
	list := transport.APIErrors{{Code: "other", Message: "foo"}}
	err := handleBasicAPIErrors(list, s.logger)
	c.Assert(err, gc.ErrorMatches, `foo`)
}
