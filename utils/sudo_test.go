// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"os"
	"os/user"
	"path/filepath"

	gc "launchpad.net/gocheck"

	jc "launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/utils"
)

type sudoSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&sudoSuite{})

func (s *sudoSuite) TestSudoCallerIds(c *gc.C) {
	s.PatchEnvironment("SUDO_UID", "0")
	s.PatchEnvironment("SUDO_GID", "0")
	for _, test := range []struct {
		uid         string
		gid         string
		errString   string
		expectedUid int
		expectedGid int
	}{{
		uid: "",
		gid: "",
	}, {
		uid:         "1001",
		gid:         "1002",
		expectedUid: 1001,
		expectedGid: 1002,
	}, {
		uid:       "1001",
		gid:       "foo",
		errString: `invalid value "foo" for SUDO_GID`,
	}, {
		uid:       "foo",
		gid:       "bar",
		errString: `invalid value "foo" for SUDO_UID`,
	}} {
		os.Setenv("SUDO_UID", test.uid)
		os.Setenv("SUDO_GID", test.gid)
		uid, gid, err := utils.SudoCallerIds()
		if test.errString == "" {
			c.Assert(err, gc.IsNil)
			c.Assert(uid, gc.Equals, test.expectedUid)
			c.Assert(gid, gc.Equals, test.expectedGid)
		} else {
			c.Assert(err, gc.ErrorMatches, test.errString)
			c.Assert(uid, gc.Equals, 0)
			c.Assert(gid, gc.Equals, 0)
		}
	}
}

func (s *sudoSuite) TestMkDirForUserAsUser(c *gc.C) {
	dir := filepath.Join(c.MkDir(), "new-dir")
	err := utils.MkdirForUser(dir, 0755)
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) setupBadSudoUid() {
	s.PatchEnvironment("SUDO_UID", "omg")
	s.PatchEnvironment("SUDO_GID", "omg")
	s.PatchValue(&utils.CheckIfRoot, func() bool { return true })
}

func (s *sudoSuite) setupGoodSudoUid(c *gc.C) {
	user, err := user.Current()
	c.Assert(err, gc.IsNil)
	s.PatchEnvironment("SUDO_UID", user.Uid)
	s.PatchEnvironment("SUDO_GID", user.Gid)
	s.PatchValue(&utils.CheckIfRoot, func() bool { return true })
}

func (s *sudoSuite) TestMkDirForUserRoot(c *gc.C) {
	s.setupGoodSudoUid(c)
	dir := filepath.Join(c.MkDir(), "new-dir")
	err := utils.MkdirForUser(dir, 0755)
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) TestMkDirForUserWithError(c *gc.C) {
	s.setupBadSudoUid()
	dir := filepath.Join(c.MkDir(), "new-dir")
	err := utils.MkdirForUser(dir, 0755)
	c.Assert(err, gc.ErrorMatches, `invalid value "omg" for SUDO_UID`)
	// The directory is still there.
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) TestMkDirAllForUserAsUser(c *gc.C) {
	dir := filepath.Join(c.MkDir(), "new-dir", "and-another")
	err := utils.MkdirAllForUser(dir, 0755)
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) TestMkDirAllForUserRoot(c *gc.C) {
	s.setupGoodSudoUid(c)

	dir := filepath.Join(c.MkDir(), "new-dir", "and-another")
	err := utils.MkdirAllForUser(dir, 0755)
	c.Assert(err, gc.IsNil)
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) TestMkDirAllForUserWithError(c *gc.C) {
	s.setupBadSudoUid()

	base := c.MkDir()
	dir := filepath.Join(base, "new-dir", "and-another")
	err := utils.MkdirAllForUser(dir, 0755)
	c.Assert(err, gc.ErrorMatches, `invalid value "omg" for SUDO_UID`)
	// The directory is still there.
	c.Assert(dir, jc.IsDirectory)
}

func (s *sudoSuite) TestChownToUserNotRoot(c *gc.C) {
	// Just in case someone runs the suite as root, we don't want to fail.
	s.PatchValue(&utils.CheckIfRoot, func() bool { return false })

	nonExistant := filepath.Join(c.MkDir(), "non-existant")
	err := utils.ChownToUser(nonExistant)
	c.Assert(err, gc.IsNil)
}

func (s *sudoSuite) TestChownToUserBadUid(c *gc.C) {
	s.setupBadSudoUid()
	err := utils.ChownToUser(c.MkDir())
	c.Assert(err, gc.ErrorMatches, `invalid value "omg" for SUDO_UID`)
}

func (s *sudoSuite) TestChownToUser(c *gc.C) {
	s.setupGoodSudoUid(c)
	err := utils.ChownToUser(c.MkDir(), c.MkDir())
	c.Assert(err, gc.IsNil)
}
