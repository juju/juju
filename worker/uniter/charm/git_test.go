package charm_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/charm"
	"os"
	"path/filepath"
)

type GitDirSuite struct {
	testing.LoggingSuite
	oldLcAll string
}

var _ = Suite(&GitDirSuite{})

func (s *GitDirSuite) SetUpTest(c *C) {
	s.LoggingSuite.SetUpTest(c)
	s.oldLcAll = os.Getenv("LC_ALL")
	os.Setenv("LC_ALL", "en_US")
}

func (s *GitDirSuite) TearDownTest(c *C) {
	os.Setenv("LC_ALL", s.oldLcAll)
	s.LoggingSuite.TearDownTest(c)
}

func (s *GitDirSuite) TestCreate(c *C) {
	base := c.MkDir()
	repo := charm.NewGitDir(filepath.Join(base, "repo"))
	exists, err := repo.Exists()
	c.Assert(err, IsNil)
	c.Assert(exists, Equals, false)

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

	err = repo.Init()
	c.Assert(err, IsNil)
}

func (s *GitDirSuite) TestAddCommitPullRevert(c *C) {
	repo := newRepo(c)
	base := c.MkDir()
	other := charm.NewGitDir(filepath.Join(base, "other"))
	err := other.Init()
	c.Assert(err, IsNil)
	err = ioutil.WriteFile(filepath.Join(other.Path(), "initial"), []byte("initial"), 0644)
	c.Assert(err, IsNil)
	err = other.AddAll()
	c.Assert(err, IsNil)
	err = other.Commitf("initial")
	c.Assert(err, IsNil)

	err = other.Pull(repo)
	c.Assert(err, IsNil)
	fi, err := os.Stat(filepath.Join(other.Path(), "some-dir"))
	c.Assert(err, IsNil)
	c.Assert(fi.IsDir(), Equals, true)
	data, err := ioutil.ReadFile(filepath.Join(other.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "hello")

	err = other.Revert()
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(other.Path(), "some-file"))
	c.Assert(os.IsNotExist(err), Equals, true)
	_, err = os.Stat(filepath.Join(other.Path(), "some-dir"))
	c.Assert(os.IsNotExist(err), Equals, true)
	data, err = ioutil.ReadFile(filepath.Join(other.Path(), "initial"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "initial")
}

func (s *GitDirSuite) TestClone(c *C) {
	repo := newRepo(c)
	base := c.MkDir()
	other, err := repo.Clone(filepath.Join(base, "other"))
	c.Assert(err, IsNil)
	_, err = os.Stat(filepath.Join(other.Path(), "some-file"))
	c.Assert(os.IsNotExist(err), Equals, true)
	_, err = os.Stat(filepath.Join(other.Path(), "some-dir"))
	c.Assert(os.IsNotExist(err), Equals, true)

	err = other.AddAll()
	c.Assert(err, IsNil)
	err = other.Commitf("blank overwrite")
	c.Assert(err, IsNil)

	lines, err := other.Log()
	c.Assert(err, IsNil)
	c.Assert(lines, HasLen, 2)
	c.Assert(lines[0], Matches, "[a-f0-9]{7} blank overwrite")
	c.Assert(lines[1], Matches, "[a-f0-9]{7} im in ur repo committin ur files")
}

func (s *GitDirSuite) TestRecover(c *C) {
	repo := newRepo(c)
	err := ioutil.WriteFile(filepath.Join(repo.Path(), "some-file"), []byte("goodbye"), 0644)
	c.Assert(err, IsNil)

	err = repo.Recover()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(filepath.Join(repo.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "goodbye")

	err = ioutil.WriteFile(filepath.Join(repo.Path(), ".git", "index.lock"), nil, 0644)
	c.Assert(err, IsNil)

	err = repo.Recover()
	c.Assert(err, IsNil)
	data, err = ioutil.ReadFile(filepath.Join(repo.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "goodbye")

	err = repo.AddAll()
	c.Assert(err, IsNil)
	err = repo.Commitf("blah")
	c.Assert(err, IsNil)
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
