// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"io/ioutil"
	"os/user"
	"path/filepath"

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
	currentUser, err := user.Current()
	c.Assert(err, gc.IsNil)
	for _, test := range []struct {
		path     string
		expected string
		err      string
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
		path:     "~",
		expected: "/home/test-user",
	}, {
		path:     "~" + currentUser.Username,
		expected: currentUser.HomeDir,
	}, {
		path:     "~" + currentUser.Username + "/foo",
		expected: currentUser.HomeDir + "/foo",
	}, {
		path:     "~" + currentUser.Username + "/foo//../bar",
		expected: currentUser.HomeDir + "/bar",
	}, {
		path: "~foobar/path",
		err:  "user: unknown user foobar",
	}} {
		actual, err := utils.NormalizePath(test.path)
		if test.err != "" {
			c.Check(err, gc.ErrorMatches, test.err)
		} else {
			c.Check(err, gc.IsNil)
			c.Check(actual, gc.Equals, test.expected)
		}
	}
}

func (*fileSuite) TestCopyFile(c *gc.C) {
	dir := c.MkDir()
	f, err := ioutil.TempFile(dir, "source")
	c.Assert(err, gc.IsNil)
	defer f.Close()
	_, err = f.Write([]byte("hello world"))
	c.Assert(err, gc.IsNil)
	dest := filepath.Join(dir, "dest")

	err = utils.CopyFile(dest, f.Name())
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(dest)
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "hello world")
}
