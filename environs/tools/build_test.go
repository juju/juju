// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	exttest "github.com/juju/testing"
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
	exttest.PatchExecHelper
}

var _ = gc.Suite(&buildSuite{})

func (b *buildSuite) SetUpTest(c *gc.C) {
	b.BaseSuite.SetUpTest(c)
	dir1 := c.MkDir()
	dir2 := c.MkDir()

	// Ensure we don't look in the real /usr/lib/juju for jujud-versions.yaml.
	b.PatchValue(&tools.VersionFileFallbackDir, c.MkDir())

	c.Log(dir1)
	c.Log(dir2)

	path := os.Getenv("PATH")
	os.Setenv("PATH", strings.Join([]string{dir1, dir2, path}, string(filepath.ListSeparator)))

	// Make an executable file called "juju-test" in dir2.
	b.filePath = filepath.Join(dir2, "juju-test")
	err := os.WriteFile(
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

func (b *buildSuite) TestEmptyArchive(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	err := tools.Archive(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, jc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, gc.Equals, io.EOF)
}

func (b *buildSuite) TestArchiveAndSHA256(c *gc.C) {
	var buf bytes.Buffer
	dir := c.MkDir()
	sha256hash, err := tools.ArchiveAndSHA256(&buf, dir)
	c.Assert(err, jc.ErrorIsNil)

	h := sha256.New()
	h.Write(buf.Bytes())
	c.Assert(sha256hash, gc.Equals, fmt.Sprintf("%x", h.Sum(nil)))

	gzr, err := gzip.NewReader(&buf)
	c.Assert(err, jc.ErrorIsNil)
	r := tar.NewReader(gzr)
	_, err = r.Next()
	c.Assert(err, gc.Equals, io.EOF)
}

func (b *buildSuite) setUpFakeBinaries(c *gc.C, versionFile string) string {
	dir := c.MkDir()
	err := os.WriteFile(filepath.Join(dir, "juju"), []byte("some data"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dir, "jujuc"), []byte(fakeBinary), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = os.WriteFile(filepath.Join(dir, "jujud"), []byte(fakeBinary), 0755)
	c.Assert(err, jc.ErrorIsNil)
	if versionFile != "" {
		err = os.WriteFile(filepath.Join(dir, "jujud-versions.yaml"), []byte(versionFile), 0755)
		c.Assert(err, jc.ErrorIsNil)
	}

	// Mock out args[0] so that copyExistingJujus can find our fake
	// binary. Tricky - we need to copy the test binary into the
	// directory so patching out exec can work.
	oldArg0 := os.Args[0]
	testBinary := filepath.Join(dir, "tst")
	os.Args[0] = testBinary
	err = os.Link(oldArg0, testBinary)
	if _, ok := err.(*os.LinkError); ok {
		// Soft link when cross device.
		err = os.Symlink(oldArg0, testBinary)
	}
	c.Assert(err, jc.ErrorIsNil)
	b.AddCleanup(func(c *gc.C) {
		os.Args[0] = oldArg0
	})
	return dir
}

const (
	fakeBinary = "some binary content\n"
)
