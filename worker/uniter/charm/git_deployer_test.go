// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	gc "launchpad.net/gocheck"

	corecharm "launchpad.net/juju-core/charm"
	"launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/worker/uniter/charm"
)

type DeployerSuite struct {
	testing.GitSuite
	bundles    *bundleReader
	targetPath string
	deployer   charm.Deployer
}

var _ = gc.Suite(&DeployerSuite{})

func (s *DeployerSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.bundles = &bundleReader{}
	s.targetPath = filepath.Join(c.MkDir(), "target")
	deployerPath := filepath.Join(c.MkDir(), "deployer")
	s.deployer = charm.NewGitDeployer(s.targetPath, deployerPath, s.bundles)
}

func (s *DeployerSuite) TestUnsetCharm(c *gc.C) {
	err := s.deployer.Deploy()
	c.Assert(err, gc.ErrorMatches, "charm deployment failed: no charm set")
}

func (s *DeployerSuite) TestInstall(c *gc.C) {
	// Prepare.
	info := s.bundles.Add(c, corecharm.MustParseURL("cs:s/c-1"), func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, gc.IsNil)
	})
	err := s.deployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)
	checkCleanup(c, s.deployer)

	// Install.
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)
	checkCleanup(c, s.deployer)

	// Check content.
	data, err := ioutil.ReadFile(filepath.Join(s.targetPath, "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "hello")

	target := charm.NewGitDir(s.targetPath)
	url, err := charm.ReadCharmURL(target)
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.DeepEquals, corecharm.MustParseURL("cs:s/c-1"))
	lines, err := target.Log()
	c.Assert(err, gc.IsNil)
	c.Assert(lines, gc.HasLen, 2)
	c.Assert(lines[0], gc.Matches, `[0-9a-f]{7} Deployed charm "cs:s/c-1".`)
	c.Assert(lines[1], gc.Matches, `[0-9a-f]{7} Imported charm "cs:s/c-1" from ".*".`)
}

func (s *DeployerSuite) TestUpgrade(c *gc.C) {
	// Install.
	info1 := s.bundles.Add(c, corecharm.MustParseURL("cs:s/c-1"), func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, gc.IsNil)
		err = os.Symlink("./some-file", filepath.Join(path, "a-symlink"))
		c.Assert(err, gc.IsNil)
	})
	err := s.deployer.Stage(info1, nil)
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)

	// Upgrade.
	info2 := s.bundles.Add(c, corecharm.MustParseURL("cs:s/c-2"), func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("goodbye"), 0644)
		c.Assert(err, gc.IsNil)
		err = ioutil.WriteFile(filepath.Join(path, "a-symlink"), []byte("not any more!"), 0644)
		c.Assert(err, gc.IsNil)
	})
	err = s.deployer.Stage(info2, nil)
	c.Assert(err, gc.IsNil)
	checkCleanup(c, s.deployer)
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)
	checkCleanup(c, s.deployer)

	// Check content.
	data, err := ioutil.ReadFile(filepath.Join(s.targetPath, "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "goodbye")
	data, err = ioutil.ReadFile(filepath.Join(s.targetPath, "a-symlink"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "not any more!")

	target := charm.NewGitDir(s.targetPath)
	url, err := charm.ReadCharmURL(target)
	c.Assert(err, gc.IsNil)
	c.Assert(url, gc.DeepEquals, corecharm.MustParseURL("cs:s/c-2"))
	lines, err := target.Log()
	c.Assert(err, gc.IsNil)
	c.Assert(lines, gc.HasLen, 5)
	c.Assert(lines[0], gc.Matches, `[0-9a-f]{7} Upgraded charm to "cs:s/c-2".`)
}

func (s *DeployerSuite) TestConflictRevertResolve(c *gc.C) {
	// Install.
	info1 := s.bundles.Add(c, corecharm.MustParseURL("cs:s/c-1"), func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("hello"), 0644)
		c.Assert(err, gc.IsNil)
	})
	err := s.deployer.Stage(info1, nil)
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)

	// Mess up target.
	err = ioutil.WriteFile(filepath.Join(s.targetPath, "some-file"), []byte("mu!"), 0644)
	c.Assert(err, gc.IsNil)

	// Upgrade.
	info2 := s.bundles.Add(c, corecharm.MustParseURL("cs:s/c-2"), func(path string) {
		err := ioutil.WriteFile(filepath.Join(path, "some-file"), []byte("goodbye"), 0644)
		c.Assert(err, gc.IsNil)
	})
	err = s.deployer.Stage(info2, nil)
	c.Assert(err, gc.IsNil)
	err = s.deployer.Deploy()
	c.Assert(err, gc.Equals, charm.ErrConflict)
	checkCleanup(c, s.deployer)

	// Check state.
	target := charm.NewGitDir(s.targetPath)
	conflicted, err := target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, true)

	// Revert and check initial content.
	err = s.deployer.NotifyRevert()
	c.Assert(err, gc.IsNil)
	data, err := ioutil.ReadFile(filepath.Join(s.targetPath, "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "mu!")
	conflicted, err = target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, false)

	// Try to upgrade again.
	err = s.deployer.Deploy()
	c.Assert(err, gc.Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, true)
	checkCleanup(c, s.deployer)

	// And again.
	err = s.deployer.Deploy()
	c.Assert(err, gc.Equals, charm.ErrConflict)
	conflicted, err = target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, true)
	checkCleanup(c, s.deployer)

	// Manually resolve, and commit.
	err = ioutil.WriteFile(filepath.Join(target.Path(), "some-file"), []byte("nu!"), 0644)
	c.Assert(err, gc.IsNil)
	err = s.deployer.NotifyResolved()
	c.Assert(err, gc.IsNil)
	conflicted, err = target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, false)

	// Try a final upgrade to the same charm and check it doesn't write anything
	// except the upgrade log line.
	err = s.deployer.Deploy()
	c.Assert(err, gc.IsNil)
	checkCleanup(c, s.deployer)

	data, err = ioutil.ReadFile(filepath.Join(target.Path(), "some-file"))
	c.Assert(err, gc.IsNil)
	c.Assert(string(data), gc.Equals, "nu!")
	conflicted, err = target.Conflicted()
	c.Assert(err, gc.IsNil)
	c.Assert(conflicted, gc.Equals, false)
	lines, err := target.Log()
	c.Assert(err, gc.IsNil)
	c.Assert(lines[0], gc.Matches, `[0-9a-f]{7} Upgraded charm to "cs:s/c-2".`)
}

func bundle(c *gc.C, customize func(path string)) *corecharm.Bundle {
	base := c.MkDir()
	dirpath := testing.Charms.ClonedDirPath(base, "dummy")
	customize(dirpath)
	dir, err := corecharm.ReadDir(dirpath)
	c.Assert(err, gc.IsNil)
	bunpath := filepath.Join(base, "bundle")
	file, err := os.Create(bunpath)
	c.Assert(err, gc.IsNil)
	defer file.Close()
	err = dir.BundleTo(file)
	c.Assert(err, gc.IsNil)
	bundle, err := corecharm.ReadBundle(bunpath)
	c.Assert(err, gc.IsNil)
	return bundle
}

func checkCleanup(c *gc.C, d charm.Deployer) {
	// Only one update dir should exist and be pointed to by the 'current'
	// symlink since extra ones should have been cleaned up by
	// cleanupOrphans.
	deployerPath := charm.GitDeployerDataPath(d)
	updateDirs, err := filepath.Glob(filepath.Join(deployerPath, "update-*"))
	c.Assert(err, gc.IsNil)
	c.Assert(updateDirs, gc.HasLen, 1)
	deployerCurrent := charm.GitDeployerCurrent(d)
	current, err := os.Readlink(deployerCurrent.Path())
	c.Assert(err, gc.IsNil)
	c.Assert(updateDirs[0], gc.Equals, current)

	// No install dirs should be left behind since the one created is
	// renamed to the target path.
	installDirs, err := filepath.Glob(filepath.Join(deployerPath, "install-*"))
	c.Assert(err, gc.IsNil)
	c.Assert(installDirs, gc.HasLen, 0)
}

type bundleReader struct {
	bundles map[string]*corecharm.Bundle
}

// Read implements the BundleReader interface.
func (br *bundleReader) Read(info charm.BundleInfo, abort <-chan struct{}) (*corecharm.Bundle, error) {
	bundle, ok := br.bundles[info.URL().String()]
	if !ok {
		return nil, fmt.Errorf("no such charm!")
	}
	return bundle, nil
}

func (br *bundleReader) Add(c *gc.C, url *corecharm.URL, customize func(path string)) charm.BundleInfo {
	bundle := bundle(c, customize)
	if br.bundles == nil {
		br.bundles = map[string]*corecharm.Bundle{}
	}
	br.bundles[url.String()] = bundle
	return &bundleInfo{nil, url}
}

type bundleInfo struct {
	charm.BundleInfo
	url *corecharm.URL
}

func (info *bundleInfo) URL() *corecharm.URL {
	return info.url
}
