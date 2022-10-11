// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/charm"
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
		Name:        "architecture and channel, no os name",
		Value:       "amd64/18.04",
		ExpectedErr: `channel without os name in platform "amd64/18.04" not valid`,
	}, {
		Name:  "architecture",
		Value: "amd64",
		Expected: charm.Platform{
			Architecture: "amd64",
		},
	}, {
		Name:  "architecture, os and series",
		Value: "amd64/os/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Channel:      "series",
		},
	}, {
		Name:  "architecture, os, version and risk",
		Value: "amd64/os/version/risk",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "os",
			Channel:      "version/risk",
		},
	}, {
		Name:  "architecture, unknown os and series",
		Value: "amd64/unknown/series",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Channel:      "series",
		},
	}, {
		Name:  "architecture, unknown os and unknown series",
		Value: "amd64/unknown/unknown",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Channel:      "",
		},
	}, {
		Name:  "architecture and unknown series",
		Value: "amd64/unknown",
		Expected: charm.Platform{
			Architecture: "amd64",
			OS:           "",
			Channel:      "",
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
