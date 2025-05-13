// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"os"
	"path/filepath"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm"
	charmtesting "github.com/juju/juju/internal/charm/testing"
	"github.com/juju/juju/testcharms"
)

type charmPathSuite struct {
	repoPath string
}

var _ = tc.Suite(&charmPathSuite{})

func (s *charmPathSuite) SetUpTest(c *tc.C) {
	s.repoPath = c.MkDir()
}

func (s *charmPathSuite) cloneCharmDir(path, name string) string {
	return testcharms.Repo.ClonedDirPath(path, name)
}

func (s *charmPathSuite) TestNoPath(c *tc.C) {
	_, _, err := corecharm.NewCharmAtPath("")
	c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
}

func (s *charmPathSuite) TestInvalidPath(c *tc.C) {
	_, _, err := corecharm.NewCharmAtPath("/foo")
	c.Assert(err, tc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRepoURL(c *tc.C) {
	_, _, err := corecharm.NewCharmAtPath("ch:foo")
	c.Assert(err, tc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestInvalidRelativePath(c *tc.C) {
	_, _, err := corecharm.NewCharmAtPath("./foo")
	c.Assert(err, tc.Equals, os.ErrNotExist)
}

func (s *charmPathSuite) TestRelativePath(c *tc.C) {
	s.cloneCharmDir(s.repoPath, "mysql")
	cwd, err := os.Getwd()
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = os.Chdir(cwd) }()
	c.Assert(os.Chdir(s.repoPath), tc.ErrorIsNil)
	_, _, err = corecharm.NewCharmAtPath("mysql")
	c.Assert(corecharm.IsInvalidPathError(err), tc.IsTrue)
}

func (s *charmPathSuite) TestNoCharmAtPath(c *tc.C) {
	_, _, err := corecharm.NewCharmAtPath(c.MkDir())
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *charmPathSuite) TestCharmFromDirectoryNotSupported(c *tc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	_, _, err := corecharm.NewCharmAtPath(charmDir)
	c.Assert(err, tc.ErrorIs, coreerrors.NotSupported)
}

func (s *charmPathSuite) TestCharmArchive(c *tc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	chDir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	dir := c.MkDir()
	archivePath := filepath.Join(dir, "archive.charm")
	file, err := os.Create(archivePath)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	err = chDir.ArchiveTo(file)
	c.Assert(err, tc.ErrorIsNil)

	ch, url, err := corecharm.NewCharmAtPath(archivePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.Meta().Name, tc.Equals, "dummy")
	c.Assert(ch.Revision(), tc.Equals, 1)
	c.Assert(url, tc.DeepEquals, charm.MustParseURL("local:dummy-1"))
}

func (s *charmPathSuite) TestCharmWithManifest(c *tc.C) {
	repo := testcharms.RepoForSeries("focal")
	chDir := repo.CharmDir("cockroach")

	dir := c.MkDir()
	archivePath := filepath.Join(dir, "archive.charm")
	file, err := os.Create(archivePath)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	err = chDir.ArchiveTo(file)
	c.Assert(err, tc.ErrorIsNil)

	ch, url, err := corecharm.NewCharmAtPath(archivePath)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ch.Meta().Name, tc.Equals, "cockroachdb")
	c.Assert(ch.Revision(), tc.Equals, 0)
	c.Assert(url, tc.DeepEquals, charm.MustParseURL("local:cockroachdb-0"))
}

func (s *charmPathSuite) TestFindsSymlinks(c *tc.C) {
	charmDir := filepath.Join(s.repoPath, "dummy")
	s.cloneCharmDir(s.repoPath, "dummy")
	chDir, err := charmtesting.ReadCharmDir(charmDir)
	c.Assert(err, tc.ErrorIsNil)

	dir := c.MkDir()
	archivePath := filepath.Join(dir, "archive.charm")
	file, err := os.Create(archivePath)
	c.Assert(err, tc.ErrorIsNil)
	defer file.Close()

	err = chDir.ArchiveTo(file)
	c.Assert(err, tc.ErrorIsNil)

	charmsPath := c.MkDir()
	linkPath := filepath.Join(charmsPath, "dummy")
	err = os.Symlink(archivePath, linkPath)
	c.Assert(err, tc.IsNil)

	ch, url, err := corecharm.NewCharmAtPath(filepath.Join(charmsPath, "dummy"))
	c.Assert(err, tc.IsNil)
	c.Assert(ch.Revision(), tc.Equals, 1)
	c.Assert(ch.Meta().Name, tc.Equals, "dummy")
	c.Assert(ch.Config().Options["title"].Default, tc.Equals, "My Title")
	c.Assert(ch.(*charm.CharmArchive).Path, tc.Equals, linkPath)
	c.Assert(url, tc.DeepEquals, charm.MustParseURL("local:dummy-1"))
}
