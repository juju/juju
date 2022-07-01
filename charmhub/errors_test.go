// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package charmhub

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/charmhub/transport"
)

type ErrorsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&ErrorsSuite{})

func (ErrorsSuite) TestHandleBasicAPIErrors(c *gc.C) {
	var list transport.APIErrors
	err := handleBasicAPIErrors(list, &FakeLogger{})
	c.Assert(err, jc.ErrorIsNil)
}

func (ErrorsSuite) TestHandleBasicAPIErrorsNotFound(c *gc.C) {
	list := transport.APIErrors{{Code: transport.ErrorCodeNotFound, Message: "foo"}}
	err := handleBasicAPIErrors(list, &FakeLogger{})
	c.Assert(err, gc.ErrorMatches, `charm or bundle not found`)
}
