// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	"github.com/juju/charm/v11"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	_, _, err := corecharm.NewCharmAtPath("", "jammy")
	c.Assert(err, gc.ErrorMatches, "empty charm path")
}

func (s *charmPathSuite) TestInvalidPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("/foo", "jammy")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRepoURL(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("ch:foo", "jammy")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestInvalidRelativePath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath("./foo", "jammy")
	c.Assert(err, gc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRelativePath(c *gc.C) {
	s.cloneCharmDir(s.repoPath, "mysql")
	cwd, err := os.Getwd()
	c.Assert(err, jc.ErrorIsNil)
	defer func() { _ = os.Chdir(cwd) }()
	c.Assert(os.Chdir(s.repoPath), jc.ErrorIsNil)
	_, _, err = corecharm.NewCharmAtPath("mysql", "jammy")
	c.Assert(corecharm.IsInvalidPathError(err), jc.IsTrue)
}

func (s *charmPathSuite) TestNoCharmAtPath(c *gc.C) {
	_, _, err := corecharm.NewCharmAtPath(c.MkDir(), "jammy")
	c.Assert(err, gc.ErrorMatches, "charm not found.*")
}

func (s *charmPathSuite) TestCharm(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "mysql")
	s.cloneCharmDir(s.repoPath, "mysql")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, "focal")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "mysql")
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/mysql-1"))
}

func (s *charmPathSuite) TestCharmWithManifest(c *gc.C) {
	repo := testcharms.RepoForSeries("focal")
	charmDir := repo.CharmDir("cockroach")
	ch, url, err := corecharm.NewCharmAtPath(charmDir.Path, "")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "cockroachdb")
	c.Assert(ch.Revision(), gc.Equals, 0)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/cockroach-0"))
}

func (s *charmPathSuite) TestNoSeriesSpecified(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "mysql")
	s.cloneCharmDir(s.repoPath, "mysql")
	_, _, err := corecharm.NewCharmAtPath(charmDir, "")
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *charmPathSuite) TestNoSeriesSpecifiedForceStillFails(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "mysql")
	s.cloneCharmDir(s.repoPath, "mysql")
	_, _, err := corecharm.NewCharmAtPathForceSeries(charmDir, "", true)
	c.Assert(err, gc.ErrorMatches, "series not specified and charm does not define any")
}

func (s *charmPathSuite) TestMultiSeriesDefault(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, "")
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "new-charm-with-multi-series")
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:jammy/multi-series-charmpath-7"))
}

func (s *charmPathSuite) TestMultiSeries(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPath(charmDir, "focal")
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Meta().Name, gc.Equals, "new-charm-with-multi-series")
	c.Assert(ch.Revision(), gc.Equals, 7)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:focal/multi-series-charmpath-7"))
}

func (s *charmPathSuite) TestUnsupportedSeries(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	_, _, err := corecharm.NewCharmAtPath(charmDir, "wily")
	c.Assert(err, gc.ErrorMatches, `series "wily" not supported by charm, the charm supported series are.*`)
}

func (s *charmPathSuite) TestUnsupportedSeriesNoForce(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	_, _, err := corecharm.NewCharmAtPathForceSeries(charmDir, "wily", false)
	c.Assert(err, gc.ErrorMatches, `series "wily" not supported by charm, the charm supported series are.*`)
}

func (s *charmPathSuite) TestUnsupportedSeriesForce(c *gc.C) {
	charmDir := filepath.Join(s.repoPath, "multi-series-charmpath")
	s.cloneCharmDir(s.repoPath, "multi-series-charmpath")
	ch, url, err := corecharm.NewCharmAtPathForceSeries(charmDir, "wily", true)
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

	ch, url, err := corecharm.NewCharmAtPath(filepath.Join(charmsPath, "dummy"), "quantal")
	c.Assert(err, gc.IsNil)
	c.Assert(ch.Revision(), gc.Equals, 1)
	c.Assert(ch.Meta().Name, gc.Equals, "dummy")
	c.Assert(ch.Config().Options["title"].Default, gc.Equals, "My Title")
	c.Assert(ch.(*charm.CharmDir).Path, gc.Equals, linkPath)
	c.Assert(url, gc.DeepEquals, charm.MustParseURL("local:quantal/dummy-1"))
}
