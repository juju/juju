// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"io/ioutil"
	. "launchpad.net/gocheck"
	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/charm"
	"os"
	"path/filepath"
)

type DeployerSuite struct {
	testing.GitSuite
}

var _ = Suite(&DeployerSuite{})

func (s *DeployerSuite) TestUnsetCharm(c *C) {
	d := charm.NewDeployer(filepath.Join(c.MkDir(), "deployer"))
	err := d.Deploy(charm.NewGitDir(c.MkDir()))
	c.Assert(err, ErrorMatches, "charm deployment failed: no charm set")
}

func (s *DeployerSuite) TestInstall(c *C) {
	// Install.
	d := charm.NewDeployer(filepath.Join(c.MkDir(), "deployer"))
	bun := s.bundle(c, func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, IsNil)
	})
	err := d.Stage(bun, corecharm.MustParseURL("cs:s/c-1"))
	c.Assert(err, IsNil)
	target := charm.NewGitDir(filepath.Join(c.MkDir(), "target"))
	err = d.Deploy(target)
	c.Assert(err, IsNil)

	// Check content.
	data, err := ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "hello")
	url, err := charm.ReadCharmURL(target)
	c.Assert(err, IsNil)
	c.Assert(url, DeepEquals, corecharm.MustParseURL("cs:s/c-1"))
	lines, err := target.Log()
	c.Assert(err, IsNil)
	c.Assert(lines, HasLen, 2)
	c.Assert(lines[0], Matches, `[0-9a-f]{7} Deployed charm "cs:s/c-1".`)
	c.Assert(lines[1], Matches, `[0-9a-f]{7} Imported charm "cs:s/c-1" from ".*".`)
}

func (s *DeployerSuite) TestUpgrade(c *C) {
	// Install.
	d := charm.NewDeployer(filepath.Join(c.MkDir(), "deployer"))
	bun1 := s.bundle(c, func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, IsNil)
		err = os.Symlink("./some-file", filepath.Join(path, "a-symlink"))
		c.Assert(err, IsNil)
	})
	err := d.Stage(bun1, corecharm.MustParseURL("cs:s/c-1"))
	c.Assert(err, IsNil)
	target := charm.NewGitDir(filepath.Join(c.MkDir(), "target"))
	err = d.Deploy(target)
	c.Assert(err, IsNil)

	// Upgrade.
	bun2 := s.bundle(c, func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("goodbye"), 0644)
		c.Assert(err, IsNil)
		err = ioutil.WriteFile(filepath.Join(path, "a-symlink"), []byte("not any more!"), 0644)
		c.Assert(err, IsNil)
	})
	err = d.Stage(bun2, corecharm.MustParseURL("cs:s/c-2"))
	c.Assert(err, IsNil)
	err = d.Deploy(target)
	c.Assert(err, IsNil)

	// Check content.
	data, err := ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "goodbye")
	data, err = ioutil.ReadFile(filepath.Join(target.Path(), "a-symlink"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "not any more!")
	url, err := charm.ReadCharmURL(target)
	c.Assert(err, IsNil)
	c.Assert(url, DeepEquals, corecharm.MustParseURL("cs:s/c-2"))
	lines, err := target.Log()
	c.Assert(err, IsNil)
	c.Assert(lines, HasLen, 5)
	c.Assert(lines[0], Matches, `[0-9a-f]{7} Upgraded charm to "cs:s/c-2".`)
}

func (s *DeployerSuite) TestConflict(c *C) {
	// Install.
	d := charm.NewDeployer(filepath.Join(c.MkDir(), "deployer"))
	bun1 := s.bundle(c, func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, IsNil)
	})
	err := d.Stage(bun1, corecharm.MustParseURL("cs:s/c-1"))
	c.Assert(err, IsNil)
	target := charm.NewGitDir(filepath.Join(c.MkDir(), "target"))
	err = d.Deploy(target)
	c.Assert(err, IsNil)

	// Mess up target.
	err = ioutil.WriteFile(filepath.Join(target.Path(), "some-file"), []byte("mu!"), 0644)
	c.Assert(err, IsNil)

	// Upgrade.
	bun2 := s.bundle(c, func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("goodbye"), 0644)
		c.Assert(err, IsNil)
	})
	err = d.Stage(bun2, corecharm.MustParseURL("cs:s/c-2"))
	c.Assert(err, IsNil)
	err = d.Deploy(target)
	c.Assert(err, Equals, charm.ErrConflict)

	// Check state.
	conflicted, err := target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, true)

	// Revert and check initial content.
	err = target.Revert()
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "mu!")
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)

	// Try to upgrade again.
	err = d.Deploy(target)
	c.Assert(err, Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, true)

	// And again.
	err = d.Deploy(target)
	c.Assert(err, Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, true)

	// Manually resolve, and commit.
	err = ioutil.WriteFile(filepath.Join(target.Path(), "some-file"), []byte("nu!"), 0644)
	c.Assert(err, IsNil)
	err = target.Snapshotf("user resolved conflicts")
	c.Assert(err, IsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)

	// Try a final upgrade to the same charm and check it doesn't write anything
	// except the upgrade log line.
	err = d.Deploy(target)
	c.Assert(err, IsNil)
	data, err = ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, IsNil)
	c.Assert(string(data), Equals, "nu!")
	conflicted, err = target.Conflicted()
	c.Assert(err, IsNil)
	c.Assert(conflicted, Equals, false)
	lines, err := target.Log()
	c.Assert(err, IsNil)
	c.Assert(lines[0], Matches, `[0-9a-f]{7} Upgraded charm to "cs:s/c-2".`)
}

func (s *DeployerSuite) bundle(c *C, customize func(path string)) *corecharm.Bundle {
	base := c.MkDir()
	dirpath := testing.Charms.ClonedDirPath(base, "dummy")
	customize(dirpath)
	dir, err := corecharm.ReadDir(dirpath)
	c.Assert(err, IsNil)
	bunpath := filepath.Join(base, "bundle")
	file, err := os.Create(bunpath)
	c.Assert(err, IsNil)
	defer file.Close()
	err = dir.BundleTo(file)
	c.Assert(err, IsNil)
	bun, err := corecharm.ReadBundle(bunpath)
	c.Assert(err, IsNil)
	return bun
}
