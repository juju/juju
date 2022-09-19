// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"github.com/juju/charm/v8"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	commoncharm "github.com/juju/juju/api/common/charm"
	"github.com/juju/juju/cmd/juju/application/utils"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

type originSuite struct{}

var _ = gc.Suite(&originSuite{})

func (*originSuite) TestDeducePlatform(c *gc.C) {
	arch := constraints.MustParse("arch=amd64")
	fallback := constraints.MustParse("arch=amd64")
	series := "focal"

	platform, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeducePlatformWithFallbackArch(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("arch=s390x")
	series := "focal"

	platform, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "s390x",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeducePlatformWithNoArch(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("cores=1")
	series := "focal"

	platform, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeducePlatformWithInvalidSeries(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("arch=amd64")
	series := "bad"

	_, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, gc.ErrorMatches, `unknown OS for series: "bad"`)
}

func (*originSuite) TestDeducePlatformWithNonUbuntuSeries(c *gc.C) {
	arch := constraints.MustParse("arch=amd64")
	fallback := constraints.MustParse("arch=amd64")
	series := "win10"

	platform, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "windows",
		Series:       "win10",
	})
}

func (*originSuite) TestDeduceOriginWithSeriesNoOS(c *gc.C) {
	channel, _ := charm.ParseChannel("stable")
	platform := corecharm.Platform{
		Architecture: "amd64",
		Series:       "focal",
	}
	curl := charm.MustParseURL("cs:ubuntu")

	obtainedOrigin, err := utils.DeduceOrigin(curl, channel, platform)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Risk:         channel.String(),
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeduceOriginWithSeriesNoOSFail(c *gc.C) {
	channel, _ := charm.ParseChannel("stable")
	platform := corecharm.Platform{
		Architecture: "amd64",
		Series:       "not-a-series",
	}
	curl := charm.MustParseURL("cs:ubuntu")

	_, err := utils.DeduceOrigin(curl, channel, platform)
	c.Assert(err, gc.ErrorMatches, ".*unknown OS for series.*")
}
