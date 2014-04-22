// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/testing"
	ft "launchpad.net/juju-core/testing/filetesting"
	"launchpad.net/juju-core/worker/uniter/charm"
)

type ConverterSuite struct {
	testing.GitSuite
	targetPath string
	dataPath   string
	bundles    *bundleReader
}

var _ = gc.Suite(&ConverterSuite{})

func (s *ConverterSuite) SetUpTest(c *gc.C) {
	s.GitSuite.SetUpTest(c)
	s.targetPath = c.MkDir()
	s.dataPath = c.MkDir()
	s.bundles = &bundleReader{}
}

func (s *ConverterSuite) TestNewDeployerCreatesManifestDeployer(c *gc.C) {
	deployer, err := charm.NewDeployer(s.targetPath, s.dataPath, s.bundles)
	c.Assert(err, gc.IsNil)
	c.Assert(deployer, jc.Satisfies, charm.IsManifestDeployer)
}

func (s *ConverterSuite) TestNewDeployerCreatesGitDeployerOnceStaged(c *gc.C) {
	gitDeployer := charm.NewGitDeployer(s.targetPath, s.dataPath, s.bundles)
	info := s.bundles.AddBundle(c, charmURL(1), mockBundle{})
	err := gitDeployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)

	deployer, err := charm.NewDeployer(s.targetPath, s.dataPath, s.bundles)
	c.Assert(err, gc.IsNil)
	c.Assert(deployer, jc.Satisfies, charm.IsGitDeployer)
}

func (s *ConverterSuite) TestConvertGitDeployerBeforeDeploy(c *gc.C) {
	gitDeployer := charm.NewGitDeployer(s.targetPath, s.dataPath, s.bundles)
	info := s.bundles.AddBundle(c, charmURL(1), mockBundle{})
	err := gitDeployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)

	deployer, err := charm.NewDeployer(s.targetPath, s.dataPath, s.bundles)
	c.Assert(err, gc.IsNil)
	err = charm.FixDeployer(&deployer)
	c.Assert(err, gc.IsNil)
	c.Assert(deployer, jc.Satisfies, charm.IsManifestDeployer)
	ft.Removed{"current"}.Check(c, s.dataPath)

	err = deployer.Stage(info, nil)
	c.Assert(err, gc.IsNil)
	err = deployer.Deploy()
	c.Assert(err, gc.IsNil)
	ft.Removed{".git"}.Check(c, s.targetPath)
}

func (s *ConverterSuite) TestConvertGitDeployerAfterDeploy(c *gc.C) {
	gitDeployer := charm.NewGitDeployer(s.targetPath, s.dataPath, s.bundles)
	info1 := s.bundles.AddBundle(c, charmURL(1), mockBundle{})
	err := gitDeployer.Stage(info1, nil)
	c.Assert(err, gc.IsNil)
	err = gitDeployer.Deploy()
	c.Assert(err, gc.IsNil)

	deployer, err := charm.NewDeployer(s.targetPath, s.dataPath, s.bundles)
	c.Assert(err, gc.IsNil)
	err = charm.FixDeployer(&deployer)
	c.Assert(err, gc.IsNil)
	c.Assert(deployer, jc.Satisfies, charm.IsManifestDeployer)
	ft.Removed{"current"}.Check(c, s.dataPath)
	ft.Dir{"manifests", 0755}.Check(c, s.dataPath)

	info2 := s.bundles.AddBundle(c, charmURL(2), mockBundle{})
	err = deployer.Stage(info2, nil)
	c.Assert(err, gc.IsNil)
	err = deployer.Deploy()
	c.Assert(err, gc.IsNil)
	ft.Removed{".git"}.Check(c, s.targetPath)
}

func (s *ConverterSuite) TestPathological(c *gc.C) {
	initial := s.bundles.AddCustomBundle(c, charmURL(1), func(path string) {
		ft.File{"common", "initial", 0644}.Create(c, path)
		ft.File{"initial", "blah", 0644}.Create(c, path)
	})
	staged := s.bundles.AddCustomBundle(c, charmURL(2), func(path string) {
		ft.File{"common", "staged", 0644}.Create(c, path)
		ft.File{"user", "badwrong", 0644}.Create(c, path)
	})
	final := s.bundles.AddCustomBundle(c, charmURL(3), func(path string) {
		ft.File{"common", "final", 0644}.Create(c, path)
		ft.File{"final", "blah", 0644}.Create(c, path)
	})

	gitDeployer := charm.NewGitDeployer(s.targetPath, s.dataPath, s.bundles)
	err := gitDeployer.Stage(initial, nil)
	c.Assert(err, gc.IsNil)
	err = gitDeployer.Deploy()
	c.Assert(err, gc.IsNil)

	preserveUser := ft.File{"user", "preserve", 0644}.Create(c, s.targetPath)
	err = gitDeployer.Stage(staged, nil)
	c.Assert(err, gc.IsNil)

	deployer, err := charm.NewDeployer(s.targetPath, s.dataPath, s.bundles)
	c.Assert(err, gc.IsNil)
	err = charm.FixDeployer(&deployer)
	c.Assert(err, gc.IsNil)

	err = deployer.Stage(final, nil)
	c.Assert(err, gc.IsNil)
	err = deployer.Deploy()
	c.Assert(err, gc.IsNil)
	ft.Removed{".git"}.Check(c, s.targetPath)
	ft.Removed{"initial"}.Check(c, s.targetPath)
	ft.Removed{"staged"}.Check(c, s.targetPath)
	ft.File{"common", "final", 0644}.Check(c, s.targetPath)
	preserveUser.Check(c, s.targetPath)
}
