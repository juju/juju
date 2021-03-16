// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type osSuite struct {
}

var _ = gc.Suite(&osSuite{})

func (s *osSuite) TestHostOS(c *gc.C) {
	os := HostOS()
	switch runtime.GOOS {
	case "windows":
		c.Assert(os, gc.Equals, Windows)
	case "darwin":
		c.Assert(os, gc.Equals, OSX)
	case "linux":
		// TODO(mjs) - this should really do more by patching out
		// osReleaseFile and testing the corner cases.
		switch os {
		case Ubuntu, CentOS, GenericLinux:
		case OpenSUSE:
			c.Assert(os, gc.Equals, OpenSUSE)
		default:
			c.Fatalf("unknown linux version: %v", os)
		}
	default:
		c.Fatalf("unsupported operating system: %v", runtime.GOOS)
	}
}

func (s *osSuite) TestEquivalentTo(c *gc.C) {
	c.Check(Ubuntu.EquivalentTo(CentOS), jc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(GenericLinux), jc.IsTrue)
	c.Check(Ubuntu.EquivalentTo(OpenSUSE), jc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(Ubuntu), jc.IsTrue)
	c.Check(GenericLinux.EquivalentTo(OpenSUSE), jc.IsTrue)
	c.Check(CentOS.EquivalentTo(CentOS), jc.IsTrue)
	c.Check(CentOS.EquivalentTo(OpenSUSE), jc.IsTrue)

	c.Check(OSX.EquivalentTo(Ubuntu), jc.IsFalse)
	c.Check(OSX.EquivalentTo(Windows), jc.IsFalse)
	c.Check(GenericLinux.EquivalentTo(OSX), jc.IsFalse)
}

func (s *osSuite) TestIsLinux(c *gc.C) {
	c.Check(Ubuntu.IsLinux(), jc.IsTrue)
	c.Check(CentOS.IsLinux(), jc.IsTrue)
	c.Check(GenericLinux.IsLinux(), jc.IsTrue)
	c.Check(OpenSUSE.IsLinux(), jc.IsTrue)

	c.Check(OSX.IsLinux(), jc.IsFalse)
	c.Check(Windows.IsLinux(), jc.IsFalse)
	c.Check(Unknown.IsLinux(), jc.IsFalse)
}
