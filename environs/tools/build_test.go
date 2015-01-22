// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/testing"
)

type buildSuite struct {
	testing.BaseSuite
	restore  func()
	cwd      string
	filePath string
}

var _ = gc.Suite(&buildSuite{})

func (b *buildSuite) SetUpTest(c *gc.C) {
	b.BaseSuite.SetUpTest(c)

	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".bat"
	}

	dir1 := c.MkDir()
	dir2 := c.MkDir()

	c.Log(dir1)
	c.Log(dir2)

	path := os.Getenv("PATH")
	os.Setenv("PATH", strings.Join([]string{dir1, dir2, path}, string(filepath.ListSeparator)))

	// Make an executable file called "juju-test" in dir2.
	b.filePath = filepath.Join(dir2, "juju-test"+suffix)
	err := ioutil.WriteFile(
		b.filePath,
		[]byte("doesn't matter, we don't execute it"),
		0755)
	c.Assert(err, jc.ErrorIsNil)

	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)

	b.cwd = c.MkDir()
	err = os.Chdir(b.cwd)
	c.Assert(err, jc.ErrorIsNil)

	b.restore = func() {
		os.Setenv("PATH", path)
		os.Chdir(cwd)
	}
}

func (b *buildSuite) TearDownTest(c *gc.C) {
	b.restore()
	b.BaseSuite.TearDownTest(c)
}

func (b *buildSuite) TestFindExecutable(c *gc.C) {

	for _, test := range []struct {
		execFile   string
		expected   string
		errorMatch string
	}{{
		execFile: filepath.Join("/", "some", "absolute", "path"),
		expected: filepath.Join("/", "some", "absolute", "path"),
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
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(result, gc.Equals, test.expected)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errorMatch)
			c.Assert(result, gc.Equals, "")
		}
	}
}

const emptyArchive = "\x1f\x8b\b\x00\x00\tn\x88\x00\xffb\x18\x05\xa3`\x14\x8cX\x00\b\x00\x00\xff\xff.\xaf\xb5\xef\x00\x04\x00\x00"

func (b *buildSuite) TestEmptyArchive(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	err := tools.Archive(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.String(), gc.Equals, emptyArchive)
}

func (b *buildSuite) TestArchiveAndSHA256(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	sha256hash, err := tools.ArchiveAndSHA256(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(buf.String(), gc.Equals, emptyArchive)

	h := sha256.New()
	h.Write([]byte(emptyArchive))
	c.Assert(sha256hash, gc.Equals, fmt.Sprintf("%x", h.Sum(nil)))
}
