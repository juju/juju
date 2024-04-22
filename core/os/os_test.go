// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"runtime"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/os/ostype"
)

type osSuite struct {
}

var _ = gc.Suite(&osSuite{})

func (s *osSuite) TestHostOS(c *gc.C) {
	os := HostOS()
	switch runtime.GOOS {
	case "windows":
		c.Assert(os, gc.Equals, ostype.Windows)
	case "darwin":
		c.Assert(os, gc.Equals, ostype.OSX)
	case "linux":
		// TODO(mjs) - this should really do more by patching out
		// osReleaseFile and testing the corner cases.
		switch os {
		case ostype.Ubuntu, ostype.CentOS, ostype.GenericLinux:
		default:
			c.Fatalf("unknown linux version: %v", os)
		}
	default:
		c.Fatalf("unsupported operating system: %v", runtime.GOOS)
	}
}
