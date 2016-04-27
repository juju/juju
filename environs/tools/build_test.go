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

	exttest "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/tools"
	"github.com/juju/juju/juju/names"
	"github.com/juju/juju/testing"
)

type buildSuite struct {
	testing.BaseSuite
	restore  func()
	cwd      string
	filePath string
	exttest.PatchExecHelper
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
	root := "/"
	if runtime.GOOS == "windows" {
		root = `C:\`
	}
	for _, test := range []struct {
		execFile   string
		expected   string
		errorMatch string
	}{{
		execFile: filepath.Join(root, "some", "absolute", "path"),
		expected: filepath.Join(root, "some", "absolute", "path"),
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

func (b *buildSuite) TestGetVersionFromJujud(c *gc.C) {
	ver := version.Binary{
		Number: version.Number{
			Major: 1,
			Minor: 2,
			Tag:   "beta",
			Patch: 1,
		},
		Series: "trusty",
		Arch:   "amd64",
	}

	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr: "hey, here's some logging you should ignore",
		Stdout: ver.String(),
		Args:   argsCh,
	})

	b.PatchValue(tools.ExecCommand, execCommand)

	v, err := tools.GetVersionFromJujud("foo")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(v, gc.Equals, ver)

	select {
	case args := <-argsCh:
		cmd := filepath.Join("foo", names.Jujud)
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithParseError(c *gc.C) {
	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr: "hey, here's some logging",
		Stdout: "oops, not a valid version",
		Args:   argsCh,
	})

	b.PatchValue(tools.ExecCommand, execCommand)

	_, err := tools.GetVersionFromJujud("foo")
	c.Assert(err, gc.ErrorMatches, `invalid version "oops, not a valid version" printed by jujud`)

	select {
	case args := <-argsCh:
		cmd := filepath.Join("foo", names.Jujud)
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}

func (b *buildSuite) TestGetVersionFromJujudWithRunError(c *gc.C) {
	argsCh := make(chan []string, 1)
	execCommand := b.GetExecCommand(exttest.PatchExecConfig{
		Stderr:   "the stderr",
		Stdout:   "the stdout",
		ExitCode: 1,
		Args:     argsCh,
	})

	b.PatchValue(tools.ExecCommand, execCommand)

	_, err := tools.GetVersionFromJujud("foo")

	cmd := filepath.Join("foo", names.Jujud)
	msg := fmt.Sprintf("cannot get version from %q: exit status 1; the stderr\nthe stdout\n", cmd)

	c.Assert(err.Error(), gc.Equals, msg)

	select {
	case args := <-argsCh:
		c.Assert(args, gc.DeepEquals, []string{cmd, "version"})
	default:
		c.Fatalf("Failed to get args sent to executable.")
	}
}
