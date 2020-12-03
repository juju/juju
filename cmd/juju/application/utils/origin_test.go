// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

type originSuite struct{}

var _ = gc.Suite(&originSuite{})

func (*originSuite) TestDeducePlatform(c *gc.C) {
	arch := constraints.MustParse("arch=amd64")
	series := "focal"

	platform, err := DeducePlatform(arch, series)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeducePlatformWithNoArch(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	series := "focal"

	platform, err := DeducePlatform(arch, series)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "",
		OS:           "ubuntu",
		Series:       "focal",
	})
}

func (*originSuite) TestDeducePlatformWithInvalidSeries(c *gc.C) {
	arch := constraints.MustParse("mem=100G")
	series := "bad"

	_, err := DeducePlatform(arch, series)
	c.Assert(err, gc.ErrorMatches, `unknown OS for series: "bad"`)
}

func (*originSuite) TestDeducePlatformWithNonUbuntuSeries(c *gc.C) {
	arch := constraints.MustParse("arch=amd64")
	series := "win10"

	platform, err := DeducePlatform(arch, series)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(platform, gc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "windows",
		Series:       "win10",
	})
}
