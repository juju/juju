// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	"github.com/juju/charm/v11"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/testcharms"
)

type charmPathSuite struct {
	repoPath string
}

var _ = gc.Suite(&charmPathSuite{})

func (s *charmPathSuite) SetUpTest(c *gc.C) {
	s.repoPath = c.MkDir()
}

func (s *charmPathSuite) cloneCharmDir(path, name string) string {
	return testcharms.Repo.ClonedDirPath(path, name)
}

func (s *charmPathSuite) TestNoPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("", base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.ErrorMatches, "empty charm path")
}

func (s *charmPathSuite) TestInvalidPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("/foo", base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRepoURL(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("ch:foo", base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestInvalidRelativePath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("./foo", base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRelativePath(c *gc.C) {
	s.cloneCharmDir(s.repoPath, "mysql")
	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = os.Chdir(cwd) }()
	c.Assert(os.Chdir(s.repoPath), jc.ErrorIsNil)
	_, _, err = corecharm.NewCharmAtPath("mysql", base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(corecharm.IsInvalidPathError(err), jc.IsTrue)
}

func (s *charmPathSuite) TestNoCharmAtPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath(c.MkDir(), base.MustParseBaseFromString("ubuntu@22.04"))
	c.Assert(err, gc.ErrorMatches, "charm not found.*")
}

func (s *charmPathSuite) TestCharm(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, base.MustParseBaseFromString("ubuntu@20.04"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/dummy-1"))
}

func (s *charmPathSuite) TestCharmWithManifest(c *gc.C) {
	repo := testcharms.RepoForSeries("focal")
	charmDir := repo.CharmDir("cockroach")
	ch, url, err := corecharm.NewCharmAtPath(charmDir.Path, base.Base{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "cockroachdb")
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/cockroach-0"))
}

func (s *charmPathSuite) TestNoBaseSpecified(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "sample-fail-no-os")
	s.cloneCharmDir(s.repoPath, "sample-fail-no-os")
	_, _, err := corecharm.NewCharmAtPath(charmDir, base.Base{})
	c.Assert(err, gc.ErrorMatches, "base not specified and charm does not define any")
}

func (s *charmPathSuite) TestNoBaseSpecifiedForceStillFails(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "sample-fail-no-os")
	s.cloneCharmDir(s.repoPath, "sample-fail-no-os")
	_, _, err := corecharm.NewCharmAtPathForceBase(charmDir, base.Base{}, true)
	c.Assert(err, gc.ErrorMatches, "base not specified and charm does not define any")
}

func (s *charmPathSuite) TestMultiBaseDefault(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, base.Base{})
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "new-charm-with-multi-series")
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:jammy/multi-series-charmpath-7"))
}

func (s *charmPathSuite) TestMultiBase(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, base.MustParseBaseFromString("ubuntu@20.04"))
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "new-charm-with-multi-series")
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/multi-series-charmpath-7"))
}

func (s *charmPathSuite) TestUnsupportedBase(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	_, _, err := corecharm.NewCharmAtPath(charmDir, base.MustParseBaseFromString("ubuntu@15.10"))
	c.Assert(err, gc.ErrorMatches, `base "ubuntu@15.10" not supported by charm, the charm supported bases are.*`)
}

func (s *charmPathSuite) TestUnsupportedBaseNoForce(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	_, _, err := corecharm.NewCharmAtPathForceBase(charmDir, base.MustParseBaseFromString("ubuntu@15.10"), false)
	c.Assert(err, gc.ErrorMatches, `base "ubuntu@15.10" not supported by charm, the charm supported bases are.*`)
}

func (s *charmPathSuite) TestUnsupportedBaseForce(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPathForceBase(charmDir, base.MustParseBaseFromString("ubuntu@15.10"), true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "new-charm-with-multi-series")
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:wily/multi-series-charmpath-7"))
}

func (s *charmPathSuite) TestFindsSymlinks(c *gc.C) {
	realPath := testcharms.Repo.ClonedDirPath(c.MkDir(), "dummy")
	charmsPath := c.MkDir()
	linkPath := filepath.Join(charmsPath, "dummy")
	err := os.Symlink(realPath, linkPath)
	c.Assert(err, gc.IsNil)

	ch, url, err := corecharm.NewCharmAtPath(filepath.Join(charmsPath, "dummy"), base.MustParseBaseFromString("ubuntu@12.10"))
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Config().Options["title"].Default, gc.Equals, "My Title")
	c.Assert(ch.(*charm.CharmDir).Path, gc.Equals, linkPath)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:quantal/dummy-1"))
}
