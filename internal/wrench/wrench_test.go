// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package wrench_test

import (
	"os"
	"path/filepath"
	"runtime"
	stdtesting "testing"

	"github.com/juju/loggo/v2"
	"github.com/juju/tc"

	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/wrench"
)

func TestPackage(t *stdtesting.T) {
	tc.TestingT(t)
}

type wrenchSuite struct {
	coretesting.BaseSuite
	wrenchDir string
	logWriter loggo.TestWriter
}

var _ = tc.Suite(&wrenchSuite{})

func (s *wrenchSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	// BaseSuite turns off wrench so restore the non-testing default.
	wrench.SetEnabled(true)

	logger := loggo.GetLogger("juju.wrench")
	oldLevel := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)

	c.Assert(loggo.RegisterWriter("wrench-tests", &s.logWriter), tc.IsNil)
	s.AddCleanup(func(*tc.C) {
		s.logWriter.Clear()
		logger.SetLogLevel(oldLevel)
		loggo.RemoveWriter("wrench-tests")
		// Ensure the wrench is turned off when these tests are done.
		wrench.SetEnabled(false)
	})
}

func (s *wrenchSuite) createWrenchDir(c *tc.C) {
	s.wrenchDir = c.MkDir()
	s.PatchValue(wrench.WrenchDir, s.wrenchDir)
}

func (s *wrenchSuite) createWrenchFile(c *tc.C, name, content string) string {
	filename := filepath.Join(s.wrenchDir, name)
	err := os.WriteFile(filename, []byte(content), 0700)
	c.Assert(err, tc.ErrorIsNil)
	return filename
}

func (s *wrenchSuite) TestIsActive(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "bar")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveWithWhitespace(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "\tbar  ")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveMultiFeatures(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "one\ntwo\nbar\n")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestIsActiveMultiFeaturesWithMixedNewlines(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "one\ntwo\r\nthree\nbar\n")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)
	s.AssertActivationLogged(c)
}

func (s *wrenchSuite) TestNotActive(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "abc")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)
	s.AssertNothingLogged(c)
}

func (s *wrenchSuite) TestNoFile(c *tc.C) {
	s.createWrenchDir(c)
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)
	s.AssertFileErrorLogged(c)
}

func (s *wrenchSuite) TestMatchInOtherCategory(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "other", "bar")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)
	s.AssertFileErrorLogged(c)
}

func (s *wrenchSuite) TestNoDirectory(c *tc.C) {
	s.PatchValue(wrench.WrenchDir, "/does/not/exist")
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)
	s.AssertDirErrorLogged(c)
}

func (s *wrenchSuite) TestFileNotOwnedByJujuUser(c *tc.C) {
	s.createWrenchDir(c)
	filename := s.createWrenchFile(c, "foo", "bar")
	s.tweakOwner(c, filename)

	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.ERROR,
		Message: `wrench file for foo/bar has incorrect ownership - ignoring ` + filename,
	}})
}

func (s *wrenchSuite) TestFilePermsTooLoose(c *tc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("Windows is not fully POSIX compliant")
	}
	s.createWrenchDir(c)
	filename := s.createWrenchFile(c, "foo", "bar")
	err := os.Chmod(filename, 0666)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.ERROR,
		Message: `wrench file for foo/bar should only be writable by owner - ignoring ` + filename,
	}})
}

func (s *wrenchSuite) TestDirectoryNotOwnedByJujuUser(c *tc.C) {
	s.createWrenchDir(c)
	s.tweakOwner(c, s.wrenchDir)

	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)

	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.ERROR,
		Message: `wrench directory has incorrect ownership - wrench functionality disabled \(.+\)`,
	}})
}

func (s *wrenchSuite) TestSetEnabled(c *tc.C) {
	s.createWrenchDir(c)
	s.createWrenchFile(c, "foo", "bar")

	// Starts enabled.
	c.Assert(wrench.IsEnabled(), tc.IsTrue)
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)

	// Disable.
	c.Assert(wrench.SetEnabled(false), tc.IsTrue)
	c.Assert(wrench.IsEnabled(), tc.IsFalse)
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsFalse)

	// Enable again.
	c.Assert(wrench.SetEnabled(true), tc.IsFalse)
	c.Assert(wrench.IsEnabled(), tc.IsTrue)
	c.Assert(wrench.IsActive("foo", "bar"), tc.IsTrue)
}

var notJujuUid = uint32(os.Getuid() + 1)

func (s *wrenchSuite) AssertActivationLogged(c *tc.C) {
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.TRACE,
		Message: `wrench for foo/bar is active`,
	}})
}

func (s *wrenchSuite) AssertNothingLogged(c *tc.C) {
	c.Assert(len(s.logWriter.Log()), tc.Equals, 0)
}

func (s *wrenchSuite) AssertFileErrorLogged(c *tc.C) {
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.TRACE,
		Message: `no wrench data for foo/bar \(ignored\): ` + fileNotFound,
	}})
}

func (s *wrenchSuite) AssertDirErrorLogged(c *tc.C) {
	mc := tc.NewMultiChecker()
	mc.AddExpr(`_[_].Level`, tc.Equals, tc.ExpectedValue)
	mc.AddExpr(`_[_].Message`, tc.Matches, tc.ExpectedValue)
	mc.AddExpr(`_[_]._`, tc.Ignore)
	c.Assert(s.logWriter.Log(), mc, []loggo.Entry{{
		Level:   loggo.TRACE,
		Message: `couldn't read wrench directory: ` + fileNotFound,
	}})
}
