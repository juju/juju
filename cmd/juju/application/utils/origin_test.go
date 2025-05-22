// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cmd/juju/application/utils"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/constraints"
)

type originSuite struct{}

func TestOriginSuite(t *testing.T) {
	tc.Run(t, &originSuite{})
}

func (*originSuite) TestMakePlatform(c *tc.C) {
	arch := constraints.MustParse("arch=amd64")
	fallback := constraints.MustParse("arch=amd64")
	base := corebase.MustParseBaseFromString("ubuntu@20.04")

	platform := utils.MakePlatform(arch, base, fallback)
	c.Assert(platform, tc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}

func (*originSuite) TestMakePlatformWithFallbackArch(c *tc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("arch=s390x")
	base := corebase.MustParseBaseFromString("ubuntu@20.04")

	platform := utils.MakePlatform(arch, base, fallback)
	c.Assert(platform, tc.DeepEquals, corecharm.Platform{
		Architecture: "s390x",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}

func (*originSuite) TestMakePlatformWithNoArch(c *tc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("cores=1")
	base := corebase.MustParseBaseFromString("ubuntu@20.04")

	platform := utils.MakePlatform(arch, base, fallback)
	c.Assert(platform, tc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "ubuntu",
		Channel:      "20.04",
	})
}

func (*originSuite) TestMakePlatformWithEmptyBase(c *tc.C) {
	arch := constraints.MustParse("mem=100G")
	fallback := constraints.MustParse("cores=1")
	base := corebase.Base{}

	platform := utils.MakePlatform(arch, base, fallback)
	c.Assert(platform, tc.DeepEquals, corecharm.Platform{
		Architecture: "amd64",
		OS:           "",
		Channel:      "",
	})
}
