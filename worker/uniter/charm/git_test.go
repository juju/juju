// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/checkers"
	"launchpad.net/juju-core/worker/uniter/charm"
	"os"
	"path/filepath"
)

var curl = corecharm.MustParseURL("cs:series/blah-blah-123")

type GitDirSuite struct {
	testing.GitSuite
	LoggingSuite testing.LoggingSuite
	oldLcAll     string
}

var _ = Suite(&GitDirSuite{})

func (s *GitDirSuite) SetUpTest(c *C) {
	s.GitSuite.SetUpTest(c)
	s.LoggingSuite.SetUpTest(c)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
}

func (s *GitDirSuite) TearDownTest(c *C) {
	os.Setenv("LC_ALL", s.oldLcAll)
	s.LoggingSuite.TearDownTest(c)
	s.GitSuite.TearDownTest(c)
}

func (s *GitDirSuite) TestCreate(c *C) {
	base := c.MkDir()
	repo := charm.NewGitDir(filepath.Join(base, "repo"))
	exists, err := repo.Exists()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = ioutil.WriteFile(repo.Path(), nil, 0644)
	c.Assert(err, IsNil)
	_, err = repo.Exists()
	c.Assert(err, ErrorMatches, `".*/repo" is not a directory`)
	err = os.Remove(repo.Path())
	c.Assert(err, IsNil)

	err = os.Chmod(base, 0555)
	c.Assert(err, IsNil)
	defer os.Chmod(base, 0755)
	err = repo.Init()
	c.Assert(err, ErrorMatches, ".* permission denied")
	exists, err = repo.Exists()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

	err = os.Chmod(base, 0755)
	c.Assert(err, IsNil)
	err = repo.Init()
	c.Assert(err, IsNil)
	exists, err = repo.Exists()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, true)

	_, err = charm.ReadCharmURL(repo)
	c.Assert(err, checkers.Satisfies, os.IsNotExist)

	err = repo.Init()
	c.Assert(err, IsNil)
}

func (s *GitDirSuite) TestAddCommitPullRevert(c *C) {
	target := charm.NewGitDir(c.MkDir())
	err := target.Init()
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(target.Path(), "initial"), []byte("initial"), 0644)
	c.Assert(err, IsNil)
	err = charm.WriteCharmURL(target, curl)
	c.Assert(err, IsNil)
	err = target.AddAll()
	c.Assert(err, IsNil)
	dirty, err := target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)
	err = target.Commitf("initial")
	c.Assert(err, IsNil)
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)

	source := newRepo(c)
	err = target.Pull(source)
	c.Assert(err, IsNil)
	url, err := charm.ReadCharmURL(target)
	c.Assert(err, IsNil)
	c.Assert(url, DeepEquals, curl)
	fi, err := os.Stat(filepath.Join(target.Path(), "some-dir"))
	c.Assert(err, IsNil)
	c.Assert(fi, checkers.Satisfies, os.FileInfo.IsDir)
	data, err := ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "hello")
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)

	err = ioutil.WriteFile(filepath.Join(target.Path(), "another-file"), []byte("blah"), 0644)
	c.Assert(err, IsNil)
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)
	err = source.AddAll()
	c.Assert(err, IsNil)
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)

	err = target.Revert()
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
	_, err = os.Stat(filepath.Join(target.Path(), "some-dir"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
	data, err = ioutil.ReadFile(filepath.Join(target.Path(), "initial"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "initial")
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)
}

func (s *GitDirSuite) TestClone(c *C) {
	repo, err := newRepo(c).Clone(c.MkDir())
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(repo.Path(), "some-file"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
	_, err = os.Stat(filepath.Join(repo.Path(), "some-dir"))
	c.Assert(err, checkers.Satisfies, os.IsNotExist)
	dirty, err := repo.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)

	err = repo.AddAll()
	c.Assert(err, IsNil)
	dirty, err = repo.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)
	err = repo.Commitf("blank overwrite")
	c.Assert(err, IsNil)
	dirty, err = repo.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)

	lines, err := repo.Log()
	c.Assert(err, IsNil)
	c.Assert(lines, HasLen, 2)
	c.Assert(lines[0], Matches, "[a-f0-9]{7} blank overwrite")
	c.Assert(lines[1], Matches, "[a-f0-9]{7} im in ur repo committin ur files")
}

func (s *GitDirSuite) TestConflictRevert(c *C) {
	source := newRepo(c)
	updated, err := source.Clone(c.MkDir())
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(updated.Path(), "some-dir"), []byte("hello"), 0644)
	c.Assert(err, IsNil)
	err = updated.Snapshotf("potential conflict src")
	c.Assert(err, IsNil)
	conflicted, err := updated.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)

	target := charm.NewGitDir(c.MkDir())
	err = target.Init()
	c.Assert(err, IsNil)
	err = target.Pull(source)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(target.Path(), "some-dir", "conflicting-file"), []byte("hello"), 0644)
	c.Assert(err, IsNil)
	err = target.Snapshotf("potential conflict dst")
	c.Assert(err, IsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)

	err = target.Pull(updated)
	c.Assert(err, Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, true)
	dirty, err := target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, true)

	err = target.Revert()
	c.Assert(err, IsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)
	dirty, err = target.Dirty()
	c.Assert(err, IsNil)
	c.Assert(dirty, Equals, false)
}

func newRepo(c *C) *charm.GitDir {
	repo := charm.NewGitDir(c.MkDir())
	err := repo.Init()
	c.Assert(err, IsNil)
	err = os.Mkdir(filepath.Join(repo.Path(), "some-dir"), 0755)
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(repo.Path(), "some-file"), []byte("hello"), 0644)
	c.Assert(err, IsNil)
	err = repo.AddAll()
	c.Assert(err, IsNil)
	err = repo.Commitf("im in ur repo committin ur %s", "files")
	c.Assert(err, IsNil)
	return repo
}
