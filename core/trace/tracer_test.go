// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package trace

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type nameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&nameSuite{})

func (nameSuite) TestNameFromFuncMethod(c *gc.C) {
	name := NameFromFunc()
	c.Assert(name, gc.Equals, Name("trace.nameSuite.TestNameFromFuncMethod"))
}
