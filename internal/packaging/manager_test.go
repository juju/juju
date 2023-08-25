// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package packaging_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&DependencyManagerTestSuite{})

type DependencyManagerTestSuite struct {
	coretesting.BaseSuite
}

func (s *DependencyManagerTestSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *DependencyManagerTestSuite) TestInstallWithCentos(c *gc.C) {
	s.assertInstallCallsCorrectBinary(c, assertParams{
		series:       "centos7",
		pkg:          "foo",
		pm:           packaging.YumPackageManager,
		expPkgBinary: "yum",
		expArgs: []string{
			"--assumeyes", "--debuglevel=1", "install", "foo",
		},
	})
}

func (s *DependencyManagerTestSuite) TestInstallWithAptOnJammy(c *gc.C) {
	s.assertInstallCallsCorrectBinary(c, assertParams{
		series:       "jammy",
		pkg:          "lxd",
		pm:           packaging.AptPackageManager,
		expPkgBinary: "apt-get",
		expArgs: []string{
			"--option=Dpkg::Options::=--force-confold",
			"--option=Dpkg::Options::=--force-unsafe-io",
			"--assume-yes", "--quiet", "install",
			"lxd",
		},
	})
}

func (s *DependencyManagerTestSuite) TestInstallWithAptOnBionic(c *gc.C) {
	s.assertInstallCallsCorrectBinary(c, assertParams{
		series:       "bionic",
		pkg:          "lxd",
		pm:           packaging.AptPackageManager,
		expPkgBinary: "apt-get",
		expArgs: []string{
			"--option=Dpkg::Options::=--force-confold",
			"--option=Dpkg::Options::=--force-unsafe-io",
			"--assume-yes", "--quiet", "install", "lxd",
		},
	})
}

func (s *DependencyManagerTestSuite) TestInstallWithSnapOnDisco(c *gc.C) {
	s.assertInstallCallsCorrectBinary(c, assertParams{
		series:       "disco",
		pkg:          "foo",
		pm:           packaging.SnapPackageManager,
		expPkgBinary: "snap", // cosmic and beyond default to snap
		expArgs: []string{
			"install", "foo",
		},
	})
}

type assertParams struct {
	series       string
	pkg          string
	pm           packaging.PackageManagerName
	expPkgBinary string
	expArgs      []string
}

func (s *DependencyManagerTestSuite) assertInstallCallsCorrectBinary(c *gc.C, params assertParams) {
	testing.PatchExecutableAsEchoArgs(c, s, params.expPkgBinary)

	err := packaging.InstallDependency(fakeDep{
		pkgs: packaging.MakePackageList(params.pm, "", params.pkg),
	}, params.series)
	c.Assert(err, gc.IsNil)
	testing.AssertEchoArgs(c, params.expPkgBinary, params.expArgs...)
}

type fakeDep struct {
	pkgs []packaging.Package
}

func (f fakeDep) PackageList(_ string) ([]packaging.Package, error) {
	return f.pkgs, nil
}
