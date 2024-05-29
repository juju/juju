// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/config"
)

var _ = gc.Suite(&YumSuite{})

type YumSuite struct {
	pacconfer config.PackagingConfigurer
}

func (s *YumSuite) SetUpSuite(c *gc.C) {
	s.pacconfer = config.NewYumPackagingConfigurer()
}

func (s *YumSuite) TestDefaultPackages(c *gc.C) {
	c.Assert(s.pacconfer.DefaultPackages(), gc.DeepEquals, config.CentOSDefaultPackages)
}

func (s *YumSuite) TestIsCloudArchivePackage(c *gc.C) {
	testedPacks := []string{
		"random",
		"stuff",
		"mongodb",
		"cloud-utils",
		"more",
		"random stuff",
	}

	for i, pack := range testedPacks {
		c.Logf("Test %d: package %s:", i+1, pack)
		res := s.pacconfer.IsCloudArchivePackage(pack)
		_, there := config.CloudArchivePackagesCentOS[pack]

		c.Assert(res, gc.Equals, there)
	}
}

func (s *YumSuite) TestRenderSource(c *gc.C) {
	expected, err := testedSource.RenderSourceFile(config.YumSourceTemplate)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.pacconfer.RenderSource(testedSource)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, gc.Equals, expected)
}
