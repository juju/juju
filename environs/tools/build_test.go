// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/testing/testbase"
)

type buildSuite struct {
	testbase.LoggingSuite
	restore  func()
	cwd      string
	filePath string
}

var _ = gc.Suite(&buildSuite{})

func (b *buildSuite) SetUpTest(c *gc.C) {
	b.LoggingSuite.SetUpTest(c)

	dir1 := c.MkDir()
	dir2 := c.MkDir()

	c.Log(dir1)
	c.Log(dir2)

	path := os.Getenv("PATH")
	os.Setenv("PATH", fmt.Sprintf("%s:%s:%s", dir1, dir2, path))

	// Make an executable file called "juju-test" in dir2.
	b.filePath = filepath.Join(dir2, "juju-test")
	err := ioutil.WriteFile(
		b.filePath,
		[]byte("doesn't matter, we don't execute it"),
		0755)
	c.Assert(err, gc.IsNil)

	cwd, err := os.Getwd()
	c.Assert(err, gc.IsNil)

	b.cwd = c.MkDir()
	err = os.Chdir(b.cwd)
	c.Assert(err, gc.IsNil)

	b.restore = func() {
		os.Setenv("PATH", path)
		os.Chdir(cwd)
	}
}

func (b *buildSuite) TearDownTest(c *gc.C) {
	b.restore()
	b.LoggingSuite.TearDownTest(c)
}

func (b *buildSuite) TestFindExecutable(c *gc.C) {

	for _, test := range []struct {
		execFile   string
		expected   string
		errorMatch string
	}{{
		execFile: "/some/absolute/path",
		expected: "/some/absolute/path",
	}, {
		execFile: "./foo",
		expected: filepath.Join(b.cwd, "foo"),
	}, {
		execFile: "juju-test",
		expected: b.filePath,
	}, {
		execFile:   "non-existent-exec-file",
		errorMatch: `could not find "non-existent-exec-file" in the path`,
	}} {
		result, err := tools.FindExecutable(test.execFile)
		if test.errorMatch == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(result, gc.Equals, test.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errorMatch)
			c.Assert(result, gc.Equals, "")
		}
	}
}
