// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	internalcharm "github.com/juju/juju/internal/charm"
)

type serviceSuite struct {
	baseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestEncodeChannelAndPlatform(c *gc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, &application.Channel{
		Track:  "track",
		Risk:   application.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, gc.DeepEquals, application.Platform{
		Architecture: architecture.AMD64,
		OSType:       application.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidArch(c *gc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(ch, gc.DeepEquals, &application.Channel{
		Track:  "track",
		Risk:   application.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, gc.DeepEquals, application.Platform{
		Architecture: architecture.Unknown,
		OSType:       application.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidRisk(c *gc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "blah", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, gc.ErrorMatches, `unknown risk.*`)
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidOSType(c *gc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "windows",
			Channel:      "24.04",
		},
	})
	c.Assert(err, gc.ErrorMatches, `unknown os type.*`)
}
