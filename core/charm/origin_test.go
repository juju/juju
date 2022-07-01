// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v2/core/charm"
)

type sourceSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&sourceSuite{})

func (s sourceSuite) TestMatches(c *gc.C) {
	ok := charm.Source("xxx").Matches("xxx")
	c.Assert(ok, jc.IsTrue)
}

func (s sourceSuite) TestNotMatches(c *gc.C) {
	ok := charm.Source("xxx").Matches("yyy")
	c.Assert(ok, jc.IsFalse)
}

type platformSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&platformSuite{})

func (s platformSuite) TestParsePlatform(c *gc.C) {
	tests := []struct {
		Name        string
		Value       string
		Expected    charm.Platform
		ExpectedErr string
	}{{
		Name:        "empty",
		Value:       "",
		ExpectedErr: "platform cannot be empty",
	}, {
		Name:        "empty components",
		Value:       "//",
		ExpectedErr: `architecture in platform "//" not valid`,
	}, {
		Name:        "too many components",
		Value:       "////",
		ExpectedErr: `platform is malformed and has too many components "////"`,
	}, {
		Name:  "architecture",
		Value: "amd64",
		Expected: charm.Platform{
			Architecture: "amd64",
		},
	}, {
		Name:  "architecture and series",
		Value: "amd64/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			Series:       "series",
		},
	}, {
		Name:  "architecture, os and series",
		Value: "amd64/os/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Series:       "series",
		},
	}, {
		Name:  "architecture, os, version and risk",
		Value: "amd64/os/version/risk",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Series:       "version/risk",
		},
	}, {
		Name:  "architecture, unknown os and series",
		Value: "amd64/unknown/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Series:       "series",
		},
	}, {
		Name:  "architecture, unknown os and unknown series",
		Value: "amd64/unknown/unknown",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Series:       "",
		},
	}, {
		Name:  "architecture and unknown series",
		Value: "amd64/unknown",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Series:       "",
		},
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		ch, err := charm.ParsePlatformNormalize(test.Value)
		if test.ExpectedErr != "" {
			c.Assert(err, gc.ErrorMatches, test.ExpectedErr)
		} else {
			c.Assert(ch, gc.DeepEquals, test.Expected)
			c.Assert(err, gc.IsNil)
		}
	}
}

func (s platformSuite) TestString(c *gc.C) {
	tests := []struct {
		Name     string
		Value    string
		Expected string
	}{{
		Name:     "architecture",
		Value:    "amd64",
		Expected: "amd64",
	}, {
		Name:     "architecture and series",
		Value:    "amd64/series",
		Expected: "amd64/series",
	}, {
		Name:     "architecture, os and series",
		Value:    "amd64/os/series",
		Expected: "amd64/os/series",
	}, {
		Name:     "architecture, os, version and risk",
		Value:    "amd64/os/version/risk",
		Expected: "amd64/os/version/risk",
	}}
	for k, test := range tests {
		c.Logf("test %q at %d", test.Name, k)
		platform, err := charm.ParsePlatformNormalize(test.Value)
		c.Assert(err, gc.IsNil)
		c.Assert(platform.String(), gc.DeepEquals, test.Expected)
	}
}

type channelTrackSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&channelTrackSuite{})

func (*channelTrackSuite) TestChannelTrack(c *gc.C) {
	tests := []struct {
		channel string
		result  string
	}{{
		channel: "20.10",
		result:  "20.10",
	}, {
		channel: "focal",
		result:  "focal",
	}, {
		channel: "20.10/stable",
		result:  "20.10",
	}, {
		channel: "focal/stable",
		result:  "focal",
	}}

	for i, test := range tests {
		c.Logf("test %d - %s", i, test.channel)
		got, err := charm.ChannelTrack(test.channel)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, gc.Equals, test.result)
	}
}

type computeBaseChannelSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&computeBaseChannelSuite{})

func (*computeBaseChannelSuite) TestComputeBaseChannel(c *gc.C) {
	tests := []struct {
		platform charm.Platform
		result   string
	}{{
		platform: charm.Platform{OS: "centos", Series: "centos7"},
		result:   "7",
	}, {
		platform: charm.Platform{OS: "centos", Series: "centos8"},
		result:   "8",
	}, {
		platform: charm.Platform{OS: "ubuntu", Series: "20.04"},
		result:   "20.04",
	}, {
		platform: charm.Platform{OS: "ubuntu", Series: "focal"},
		result:   "20.04",
	}}

	for i, test := range tests {
		c.Logf("test %d - %s", i, test.platform)
		got := charm.ComputeBaseChannel(test.platform).Series
		c.Assert(got, gc.Equals, test.result)
	}
}

type normalisePlatformSeriesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&normalisePlatformSeriesSuite{})

func (*normalisePlatformSeriesSuite) TestComputeBaseChannel(c *gc.C) {
	tests := []struct {
		platform charm.Platform
		result   string
	}{{
		platform: charm.Platform{OS: "centos", Series: "centos7"},
		result:   "centos7",
	}, {
		platform: charm.Platform{OS: "centos", Series: "7"},
		result:   "centos7",
	}, {
		platform: charm.Platform{OS: "ubuntu", Series: "20.04"},
		result:   "20.04",
	}, {
		platform: charm.Platform{OS: "ubuntu", Series: "focal"},
		result:   "focal",
	}}

	for i, test := range tests {
		c.Logf("test %d - %s", i, test.platform)
		got := charm.NormalisePlatformSeries(test.platform).Series
		c.Assert(got, gc.Equals, test.result)
	}
}
