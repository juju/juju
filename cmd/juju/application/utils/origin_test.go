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
	"github.com/juju/juju/core/series"
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
		Channel:      "20.04",
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
		Channel:      "20.04",
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
		Channel:      "20.04",
	})
}

func (*originSuite) TestDeducePlatformWithInvalidSeries(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("arch=amd64")
	series := "bad"

	_, err := utils.DeducePlatform(arch, series, fallback)
	c.Assert(err, gc.ErrorMatches, `series "bad" not valid`)
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
		Channel:      "win10",
	})
}

func (*originSuite) TestDeduceOriginWithChannelNoOS(c *gc.C) {
	channel, _ := charm.ParseChannel("stable")
	platform := corecharm.Platform{
		Architecture: "amd64",
		Channel:      "20.04",
	}
	curl := charm.MustParseURL("cs:ubuntu")

	obtainedOrigin, err := utils.DeduceOrigin(curl, channel, platform)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtainedOrigin, gc.DeepEquals, commoncharm.Origin{
		Source:       commoncharm.OriginCharmStore,
		Risk:         channel.String(),
		Architecture: "amd64",
		Base:         series.MakeDefaultBase("ubuntu", "20.04"),
		Series:       "focal",
	})
}

func (*originSuite) TestDeduceOriginWithChannelNoOSFail(c *gc.C) {
	channel, _ := charm.ParseChannel("stable")
	platform := corecharm.Platform{
		Architecture: "amd64",
		Channel:      "not-a-series",
	}
	curl := charm.MustParseURL("cs:ubuntu")

	_, err := utils.DeduceOrigin(curl, channel, platform)
	c.Assert(err, gc.ErrorMatches, ".*unknown series.*")
}
