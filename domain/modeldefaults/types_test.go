// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modeldefaults

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type typesSuite struct{}

var _ = gc.Suite(&typesSuite{})

func (s *typesSuite) TestZeroDefaultsValue(c *gc.C) {
	val := DefaultAttributeValue{}
	c.Assert(val.Value(), gc.IsNil)
	c.Assert(val.ValueSource(), gc.Equals, "")

	has, source := val.Has("someval")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")
}

func (s *typesSuite) TestDefaultDefaultsValue(c *gc.C) {
	val := DefaultAttributeValue{Default: "test"}
	c.Assert(val.Value(), gc.DeepEquals, "test")
	c.Assert(val.ValueSource(), gc.Equals, "default")

	has, source := val.Has("test")
	c.Assert(has, jc.IsTrue)
	c.Assert(source, gc.Equals, "default")

	has, source = val.Has("noexist")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")
}

func (s *typesSuite) TestControllerDefaultsValue(c *gc.C) {
	val := DefaultAttributeValue{
		Default:    "default",
		Controller: "test",
	}
	c.Assert(val.Value(), gc.DeepEquals, "test")
	c.Assert(val.ValueSource(), gc.Equals, "controller")

	has, source := val.Has("test")
	c.Assert(has, jc.IsTrue)
	c.Assert(source, gc.Equals, "controller")

	has, source = val.Has("noexist")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")

	has, source = val.Has("default")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")
}

func (s *typesSuite) TestRegionDefaultsValue(c *gc.C) {
	val := DefaultAttributeValue{
		Default:    "default",
		Controller: "controller",
		Region:     "test",
	}
	c.Assert(val.Value(), gc.DeepEquals, "test")
	c.Assert(val.ValueSource(), gc.Equals, "region")

	has, source := val.Has("test")
	c.Assert(has, jc.IsTrue)
	c.Assert(source, gc.Equals, "region")

	has, source = val.Has("noexist")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")

	has, source = val.Has("default")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")

	has, source = val.Has("controller")
	c.Assert(has, jc.IsFalse)
	c.Assert(source, gc.Equals, "")
}
