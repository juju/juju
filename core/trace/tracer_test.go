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

func (nameSuite) TestNameFromFuncClosure(c *gc.C) {
	do := func() Name {
		return NameFromFunc()
	}
	c.Assert(do(), gc.Equals, Name("trace.nameSuite.TestNameFromFuncClosure"))
}

func (nameSuite) TestNameFromFuncFunction(c *gc.C) {
	c.Assert(do(), gc.Equals, Name("trace.nameSuite.TestNameFromFuncFunction"))
}

func (nameSuite) TestNameFromFuncWrappedFunction(c *gc.C) {
	c.Assert(nestedDo(), gc.Equals, Name("trace.nameSuite.TestNameFromFuncWrappedFunction"))
}

func do() Name {
	return NameFromFunc()
}

func nestedDo() Name {
	return do()
}
