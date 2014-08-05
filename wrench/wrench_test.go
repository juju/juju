// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wrench_test

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"syscall"
	stdtesting "testing"

	gc "launchpad.net/gocheck"

	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/wrench"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
)

func TestPackage(t *stdtesting.T) {
	gc.TestingT(t)
}

type wrenchSuite struct {
	coretesting.BaseSuite
	wrenchDir string
	logWriter loggo.TestWriter
}

var _ = gc.Suite(&wrenchSuite{})

func (s *wrenchSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	c.Assert(loggo.RegisterWriter("wrench-tests", &s.logWriter, loggo.TRACE), gc.IsNil)
	s.AddCleanup(func(*gc.C) {
		s.logWriter.Clear()
		loggo.RemoveWriter("wrench-tests")
	})
}

func (s *wrenchSuite) createWrenchDir(c *gc.C) {
	s.wrenchDir = c.MkDir()
	s.PatchValue(wrench.WrenchDir, s.wrenchDir)
}

func (s *wrenchSuite) createWrenchFile(c *gc.C, name, content string) string {
	filename := filepath.Join(s.wrenchDir, name)
	err := ioutil.WriteFile(filename, []byte(content), 0700)
	c.Assert(err, gc.IsNil)
	return filename
}

func (s *wrenchSuite) TestIsActive(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "bar")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveWithWhitespace(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "\tbar  ")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveMultiFeatures(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "one\ntwo\nbar\n")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveMultiFeaturesWithMixedNewlines(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "one\ntwo\r\nthree\nbar\n")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestNotActive(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "abc")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)
	s.AssertNothingLogged(c)
}

func (s *wrenchSuite) TestNoFile(c *gc.C) {
	s.createWrenchDir(c)
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)
	s.AssertFileErrorLogged(c)
}

func (s *wrenchSuite) TestMatchInOtherCategory(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "other", "bar")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)
	s.AssertFileErrorLogged(c)
}

func (s *wrenchSuite) TestNoDirectory(c *gc.C) {
	s.PatchValue(wrench.WrenchDir, "/does/not/exist")
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)
	s.AssertDirErrorLogged(c)
}

func (s *wrenchSuite) TestFileNotOwnedByJujuUser(c *gc.C) {
	s.createWrenchDir(c)
	filename := s.createWrenchFile(c, "foo", "bar")
	s.tweakOwner(c, filename)

	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)

	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR,
		`wrench file for foo/bar has incorrect ownership - ignoring ` + filename,
	}})
}

func (s *wrenchSuite) TestFilePermsTooLoose(c *gc.C) {
	s.createWrenchDir(c)
	filename := s.createWrenchFile(c, "foo", "bar")
	err := os.Chmod(filename, 0666)
	c.Assert(err, gc.IsNil)

	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)

	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR,
		`wrench file for foo/bar should only be writable by owner - ignoring ` + filename,
	}})
}

func (s *wrenchSuite) TestDirectoryNotOwnedByJujuUser(c *gc.C) {
	s.createWrenchDir(c)
	s.tweakOwner(c, s.wrenchDir)

	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)

	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{{
		loggo.ERROR,
		`wrench directory has incorrect ownership - wrench functionality disabled \(.+\)`,
	}})
}

func (s *wrenchSuite) TestSetEnabled(c *gc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "bar")

	// Starts enabled.
	c.Assert(wrench.IsEnabled(), jc.IsTrue)
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)

	// Disable.
	c.Assert(wrench.SetEnabled(false), jc.IsTrue)
	c.Assert(wrench.IsEnabled(), jc.IsFalse)
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsFalse)

	// Enable again.
	c.Assert(wrench.SetEnabled(true), jc.IsFalse)
	c.Assert(wrench.IsEnabled(), jc.IsTrue)
	c.Assert(wrench.IsActive("foo", "bar"), jc.IsTrue)
}

var notJujuUid = uint32(os.Getuid() + 1)

// Patch out the os.Stat call used by wrench so that a particular file
// appears to be owned by a UID that isn't Juju's UID.
func (s *wrenchSuite) tweakOwner(c *gc.C, targetPath string) {
	s.PatchValue(wrench.Stat, func(path string) (fi os.FileInfo, err error) {
		fi, err = os.Stat(path)
		if err != nil {
			return
		}
		if path == targetPath {
			statStruct, ok := fi.Sys().(*syscall.Stat_t)
			if !ok {
				c.Skip("this test only support on POSIX systems")
			}
			statStruct.Uid = notJujuUid
		}
		return
	})
}

func (s *wrenchSuite) AssertActivationLogged(c *gc.C) {
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.WARNING, `wrench for foo/bar is active`}})
}

func (s *wrenchSuite) AssertNothingLogged(c *gc.C) {
	c.Assert(len(s.logWriter.Log()), gc.Equals, 0)
}

func (s *wrenchSuite) AssertFileErrorLogged(c *gc.C) {
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.DEBUG, `no wrench data for foo/bar \(ignored\): ` +
			`stat .+: no such file or directory`}})
}

func (s *wrenchSuite) AssertDirErrorLogged(c *gc.C) {
	c.Assert(s.logWriter.Log(), jc.LogMatches, []jc.SimpleMessage{
		{loggo.DEBUG, `couldn't read wrench directory: ` +
			`stat .+: no such file or directory`}})
}
