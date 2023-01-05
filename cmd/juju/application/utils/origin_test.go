// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/application/utils"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/series"
)

type originSuite struct{}

var _ = gc.Suite(&originSuite{})

func (*originSuite) TestDeducePlatform(c *gc.C) {
	arch := constraints.MustParse("arch=amd64")
	fallback := constraints.MustParse("arch=amd64")
	base := series.MustParseBaseFromString("ubuntu@20.04")

	platform, err := utils.DeducePlatform(arch, base, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}

func (*originSuite) TestDeducePlatformWithFallbackArch(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("arch=s390x")
	base := series.MustParseBaseFromString("ubuntu@20.04")

	platform, err := utils.DeducePlatform(arch, base, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "s390x",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}

func (*originSuite) TestDeducePlatformWithNoArch(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("cores=1")
	base := series.MustParseBaseFromString("ubuntu@20.04")

	platform, err := utils.DeducePlatform(arch, base, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}
