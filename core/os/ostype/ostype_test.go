// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package ostype

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type osTypeSuite struct{}

var _ = gc.Suite(&osTypeSuite{})

func (s *osTypeSuite) TestEquivalentTo(c *gc.C) {
	c.Check(Ubuntu.EquivalentTo(CentOS), jc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(GenericLinux), jc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(Ubuntu), jc.IsTrue)
	c.Check(CentOS.EquivalentTo(CentOS), jc.IsTrue)
}

func (s *osTypeSuite) TestIsLinux(c *gc.C) {
	c.Check(Ubuntu.IsLinux(), jc.IsTrue)
	c.Check(CentOS.IsLinux(), jc.IsTrue)
	c.Check(GenericLinux.IsLinux(), jc.IsTrue)

	c.Check(Windows.IsLinux(), jc.IsFalse)
	c.Check(Unknown.IsLinux(), jc.IsFalse)

	c.Check(OSX.EquivalentTo(Ubuntu), jc.IsFalse)
	c.Check(OSX.EquivalentTo(Windows), jc.IsFalse)
	c.Check(GenericLinux.EquivalentTo(OSX), jc.IsFalse)
}
