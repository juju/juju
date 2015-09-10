// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package os

import (
	"runtime"

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
		if os != Ubuntu && os != CentOS && os != Arch {
			c.Fatalf("unknown linux version: %v", os)
		}
	default:
		c.Fatalf("unsupported operating system: %v", runtime.GOOS)
	}
}
