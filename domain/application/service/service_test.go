// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"

	"github.com/juju/tc"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/deployment"
	internalcharm "github.com/juju/juju/internal/charm"
)

type serviceSuite struct {
	baseSuite
}

func TestServiceSuite(t *testing.T) {
	tc.Run(t, &serviceSuite{})
}

func (s *serviceSuite) TestEncodeChannelAndPlatform(c *tc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "amd64",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, &deployment.Channel{
		Track:  "track",
		Risk:   deployment.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, tc.DeepEquals, deployment.Platform{
		Architecture: architecture.AMD64,
		OSType:       deployment.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidArch(c *tc.C) {
	ch, pl, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(ch, tc.DeepEquals, &deployment.Channel{
		Track:  "track",
		Risk:   deployment.RiskStable,
		Branch: "branch",
	})
	c.Check(pl, tc.DeepEquals, deployment.Platform{
		Architecture: architecture.Unknown,
		OSType:       deployment.Ubuntu,
		Channel:      "24.04",
	})
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidRisk(c *tc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "blah", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "ubuntu",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `unknown risk.*`)
}

func (s *serviceSuite) TestEncodeChannelAndPlatformInvalidOSType(c *tc.C) {
	_, _, err := encodeChannelAndPlatform(corecharm.Origin{
		Channel: ptr(internalcharm.MakePermissiveChannel("track", "stable", "branch")),
		Platform: corecharm.Platform{
			Architecture: "armhf",
			OS:           "windows",
			Channel:      "24.04",
		},
	})
	c.Assert(err, tc.ErrorMatches, `unknown os type.*`)
}
