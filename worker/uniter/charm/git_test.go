// Copyright 2012-2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package charm_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	corecharm "gopkg.in/juju/charm.v5"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/worker/uniter/charm"
)

var curl = corecharm.MustParseURL("cs:series/blah-blah-123")

type GitDirSuite struct {
	testing.GitSuite
}

var _ = gc.Suite(&GitDirSuite{})

func (s *GitDirSuite) TestInitConfig(c *gc.C) {
	base := c.MkDir()
	repo := charm.NewGitDir(filepath.Join(base, "repo"))
	err := repo.Init()
	c.Assert(err, jc.ErrorIsNil)

	cmd := exec.Command("git", "config", "--list", "--local")
	cmd.Dir = repo.Path()
	out, err := cmd.Output()
	c.Assert(err, jc.ErrorIsNil)
	outstr := string(out)
	c.Assert(outstr, jc.Contains, "user.email=juju@localhost")
	c.Assert(outstr, jc.Contains, "user.name=juju")
}

func (s *GitDirSuite) TestCreate(c *gc.C) {
	base := c.MkDir()
	repo := charm.NewGitDir(filepath.Join(base, "repo"))
	exists, err := repo.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)

	err = ioutil.WriteFile(repo.Path(), nil, 0644)
	c.Assert(err, jc.ErrorIsNil)
	_, err = repo.Exists()
	c.Assert(err, gc.ErrorMatches, `".*/repo" is not a directory`)
	err = os.Remove(repo.Path())
	c.Assert(err, jc.ErrorIsNil)

	err = os.Chmod(base, 0555)
	c.Assert(err, jc.ErrorIsNil)
	defer os.Chmod(base, 0755)
	err = repo.Init()
	c.Assert(err, gc.ErrorMatches, ".* permission denied")
	exists, err = repo.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)

	err = os.Chmod(base, 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = repo.Init()
	c.Assert(err, jc.ErrorIsNil)
	exists, err = repo.Exists()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsTrue)

	_, err = repo.ReadCharmURL()
	c.Assert(err, jc.Satisfies, os.IsNotExist)

	err = repo.Init()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *GitDirSuite) TestAddCommitPullRevert(c *gc.C) {
	target := charm.NewGitDir(c.MkDir())
	err := target.Init()
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(target.Path(), "initial"), []byte("initial"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = target.WriteCharmURL(curl)
	c.Assert(err, jc.ErrorIsNil)
	err = target.AddAll()
	c.Assert(err, jc.ErrorIsNil)
	dirty, err := target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
	err = target.Commitf("initial")
	c.Assert(err, jc.ErrorIsNil)
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)

	source := newRepo(c)
	err = target.Pull(source)
	c.Assert(err, jc.ErrorIsNil)
	url, err := target.ReadCharmURL()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(url, gc.DeepEquals, curl)
	fi, err := os.Stat(filepath.Join(target.Path(), "some-dir"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(fi, jc.Satisfies, os.FileInfo.IsDir)
	data, err := ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "hello")
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)

	err = ioutil.WriteFile(filepath.Join(target.Path(), "another-file"), []byte("blah"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
	err = source.AddAll()
	c.Assert(err, jc.ErrorIsNil)
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)

	err = target.Revert()
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	_, err = os.Stat(filepath.Join(target.Path(), "some-dir"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	data, err = ioutil.ReadFile(filepath.Join(target.Path(), "initial"))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, "initial")
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}

func (s *GitDirSuite) TestClone(c *gc.C) {
	repo, err := newRepo(c).Clone(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	_, err = os.Stat(filepath.Join(repo.Path(), "some-file"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	_, err = os.Stat(filepath.Join(repo.Path(), "some-dir"))
	c.Assert(err, jc.Satisfies, os.IsNotExist)
	dirty, err := repo.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)

	err = repo.AddAll()
	c.Assert(err, jc.ErrorIsNil)
	dirty, err = repo.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)
	err = repo.Commitf("blank overwrite")
	c.Assert(err, jc.ErrorIsNil)
	dirty, err = repo.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)

	lines, err := repo.Log()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lines, gc.HasLen, 2)
	c.Assert(lines[0], gc.Matches, "[a-f0-9]{7} blank overwrite")
	c.Assert(lines[1], gc.Matches, "[a-f0-9]{7} im in ur repo committin ur files")
}

func (s *GitDirSuite) TestConflictRevert(c *gc.C) {
	source := newRepo(c)
	updated, err := source.Clone(c.MkDir())
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(updated.Path(), "some-dir"), []byte("hello"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = updated.Snapshotf("potential conflict src")
	c.Assert(err, jc.ErrorIsNil)
	conflicted, err := updated.Conflicted()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conflicted, jc.IsFalse)

	target := charm.NewGitDir(c.MkDir())
	err = target.Init()
	c.Assert(err, jc.ErrorIsNil)
	err = target.Pull(source)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(target.Path(), "some-dir", "conflicting-file"), []byte("hello"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = target.Snapshotf("potential conflict dst")
	c.Assert(err, jc.ErrorIsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conflicted, jc.IsFalse)

	err = target.Pull(updated)
	c.Assert(err, gc.Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conflicted, jc.IsTrue)
	dirty, err := target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsTrue)

	err = target.Revert()
	c.Assert(err, jc.ErrorIsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(conflicted, jc.IsFalse)
	dirty, err = target.Dirty()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(dirty, jc.IsFalse)
}

func newRepo(c *gc.C) *charm.GitDir {
	repo := charm.NewGitDir(c.MkDir())
	err := repo.Init()
	c.Assert(err, jc.ErrorIsNil)
	err = os.Mkdir(filepath.Join(repo.Path(), "some-dir"), 0755)
	c.Assert(err, jc.ErrorIsNil)
	err = ioutil.WriteFile(filepath.Join(repo.Path(), "some-file"), []byte("hello"), 0644)
	c.Assert(err, jc.ErrorIsNil)
	err = repo.AddAll()
	c.Assert(err, jc.ErrorIsNil)
	err = repo.Commitf("im in ur repo committin ur %s", "files")
	c.Assert(err, jc.ErrorIsNil)
	return repo
}
