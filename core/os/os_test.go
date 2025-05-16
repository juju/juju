// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package os

import (
	"runtime"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/os/ostype"
)

type osSuite struct {
}

func TestOsSuite(t *stdtesting.T) { tc.Run(t, &osSuite{}) }
func (s *osSuite) TestHostOS(c *tc.C) {
	os := HostOS()
	switch runtime.GOOS {
	case "windows":
		c.Assert(os, tc.Equals, ostype.Windows)
	case "darwin":
		c.Assert(os, tc.Equals, ostype.OSX)
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
