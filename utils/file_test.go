// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/utils"
)

type fileSuite struct {
	oldHome string
}

var _ = gc.Suite(&fileSuite{})

func (s *fileSuite) SetUpTest(c *gc.C) {
	s.oldHome = osenv.Home()
	err := osenv.SetHome("/home/test-user")
	c.Assert(err, gc.IsNil)
}

func (s *fileSuite) TearDownTest(c *gc.C) {
	err := osenv.SetHome(s.oldHome)
	c.Assert(err, gc.IsNil)
}

func (*fileSuite) TestNormalizePath(c *gc.C) {
	for _, test := range []struct {
		path     string
		expected string
	}{{
		path:     "/var/lib/juju",
		expected: "/var/lib/juju",
	}, {
		path:     "~/foo",
		expected: "/home/test-user/foo",
	}, {
		path:     "~/foo//../bar",
		expected: "/home/test-user/bar",
	}, {
		path:     "~bob/foo",
		expected: "~bob/foo",
	}} {
		c.Assert(utils.NormalizePath(test.path), gc.Equals, test.expected)
	}
}
