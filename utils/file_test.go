// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type fileSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&fileSuite{})

func (*fileSuite) TestNormalizePath(c *gc.C) {
	home := c.MkDir()
	err := utils.SetHome(home)
	c.Assert(err, gc.IsNil)
	// TODO (frankban) bug 1324841: improve the isolation of this suite.
	currentUser, err := user.Current()
	c.Assert(err, gc.IsNil)
	for i, test := range []struct {
		path     string
		expected string
		err      string
	}{{
		path:     "/var/lib/juju",
		expected: "/var/lib/juju",
	}, {
		path:     "~/foo",
		expected: filepath.Join(home, "foo"),
	}, {
		path:     "~/foo//../bar",
		expected: filepath.Join(home, "bar"),
	}, {
		path:     "~",
		expected: home,
	}, {
		path:     "~" + currentUser.Username,
		expected: currentUser.HomeDir,
	}, {
		path:     "~" + currentUser.Username + "/foo",
		expected: filepath.Join(currentUser.HomeDir, "foo"),
	}, {
		path:     "~" + currentUser.Username + "/foo//../bar",
		expected: filepath.Join(currentUser.HomeDir, "bar"),
	}, {
		path: "~foobar/path",
		err:  "user: unknown user foobar",
	}} {
		c.Logf("test %d: %s", i, test.path)
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

var atomicWriteFileTests = []struct {
	summary   string
	change    func(filename string, contents []byte) error
	check     func(c *gc.C, fileInfo os.FileInfo)
	expectErr string
}{{
	summary: "atomic file write and chmod 0644",
	change: func(filename string, contents []byte) error {
		return utils.AtomicWriteFile(filename, contents, 0765)
	},
	check: func(c *gc.C, fi os.FileInfo) {
		c.Assert(fi.Mode(), gc.Equals, 0765)
	},
}, {
	summary: "atomic file write and change",
	change: func(filename string, contents []byte) error {
		chmodChange := func(f *os.File) error {
			return f.Chmod(0700)
		}
		return utils.AtomicWriteFileAndChange(filename, contents, chmodChange)
	},
	check: func(c *gc.C, fi os.FileInfo) {
		c.Assert(fi.Mode(), gc.Equals, 0700)
	},
}, {
	summary: "atomic file write empty contents",
	change: func(filename string, contents []byte) error {
		nopChange := func(*os.File) error {
			return nil
		}
		return utils.AtomicWriteFileAndChange(filename, contents, nopChange)
	},
}, {
	summary: "atomic file write and failing change func",
	change: func(filename string, contents []byte) error {
		errChange := func(*os.File) error {
			return fmt.Errorf("pow!")
		}
		return utils.AtomicWriteFileAndChange(filename, contents, errChange)
	},
	expectErr: "pow!",
}}

func (*fileSuite) TestAtomicWriteFile(c *gc.C) {
	dir := c.MkDir()
	name := "test.file"
	path := filepath.Join(dir, name)
	assertDirContents := func(names ...string) {
		fis, err := ioutil.ReadDir(dir)
		c.Assert(err, gc.IsNil)
		c.Assert(fis, gc.HasLen, len(names))
		for i, name := range names {
			c.Assert(fis[i].Name(), gc.Equals, name)
		}
	}
	assertNotExist := func(path string) {
		_, err := os.Lstat(path)
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	}

	for i, test := range atomicWriteFileTests {
		c.Logf("test %d: %s", i, test.summary)
		// First - test with file not already there.
		assertDirContents()
		assertNotExist(path)
		contents := []byte("some\ncontents")

		err := test.change(path, contents)
		if test.expectErr == "" {
			c.Assert(err, gc.IsNil)
			data, err := ioutil.ReadFile(path)
			c.Assert(err, gc.IsNil)
			c.Assert(data, jc.DeepEquals, contents)
			assertDirContents(name)
		} else {
			c.Assert(err, gc.ErrorMatches, test.expectErr)
			assertDirContents()
			continue
		}

		// Second - test with a file already there.
		contents = []byte("new\ncontents")
		err = test.change(path, contents)
		c.Assert(err, gc.IsNil)
		data, err := ioutil.ReadFile(path)
		c.Assert(err, gc.IsNil)
		c.Assert(data, jc.DeepEquals, contents)
		assertDirContents(name)

		// Remove the file to reset scenario.
		c.Assert(os.Remove(path), gc.IsNil)
	}
}
