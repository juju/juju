// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package config_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/config"
)

var _ = gc.Suite(&AptSuite{})

type AptSuite struct {
	pacconfer config.PackagingConfigurer
}

func (s *AptSuite) SetUpSuite(c *gc.C) {
	s.pacconfer = config.NewAptPackagingConfigurer()
}

func (s *AptSuite) TestDefaultPackages(c *gc.C) {
	c.Assert(s.pacconfer.DefaultPackages(), gc.DeepEquals, config.UbuntuDefaultPackages)
}

func (s *AptSuite) TestIsCloudArchivePackage(c *gc.C) {
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
		_, there := config.CloudArchivePackagesUbuntu[pack]

		c.Assert(res, gc.Equals, there)
	}
}

func (s *AptSuite) TestRenderSource(c *gc.C) {
	expected, err := testedSource.RenderSourceFile(config.AptSourceTemplate)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.pacconfer.RenderSource(testedSource)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, gc.Equals, expected)
}

func (s *AptSuite) TestRenderPreferences(c *gc.C) {
	expected, err := testedPrefs.RenderPreferenceFile(config.AptPreferenceTemplate)
	c.Assert(err, jc.ErrorIsNil)

	res, err := s.pacconfer.RenderPreferences(testedPrefs)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(res, gc.Equals, expected)
}

func (s *AptSuite) TestApplyCloudArchiveTarget(c *gc.C) {
	res := s.pacconfer.ApplyCloudArchiveTarget("some-package")
	c.Assert(res, gc.DeepEquals, []string{
		"some-package",
	})
}
