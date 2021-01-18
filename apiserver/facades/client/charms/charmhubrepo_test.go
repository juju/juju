// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charms

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	corecharm "github.com/juju/juju/core/charm"
)

type channelPlatformSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&channelPlatformSuite{})

func (s channelPlatformSuite) TestMatchChannel(c *gc.C) {
	cp := channelPlatform{
		Channel: corecharm.MustParseChannel("latest/stable"),
	}

	matched := cp.MatchChannel(transport.InfoChannelMap{
		Channel: transport.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	})
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchChannelNotMatched(c *gc.C) {
	cp := channelPlatform{
		Channel: corecharm.MustParseChannel("2.9/stable"),
	}

	matched := cp.MatchChannel(transport.InfoChannelMap{
		Channel: transport.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	})
	c.Assert(matched, jc.IsFalse)
}

func (s channelPlatformSuite) TestMatchChannelMissingTrack(c *gc.C) {
	cp := channelPlatform{
		Channel: corecharm.MustParseChannel("stable"),
	}

	matched := cp.MatchChannel(transport.InfoChannelMap{
		Channel: transport.Channel{
			Track: "latest",
			Risk:  "stable",
		},
	})
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchArchAMD64(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: "amd64",
			},
		},
	})
	c.Assert(override.Arch, jc.IsFalse)
	c.Assert(override.Series, jc.IsFalse)
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchArchAll(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: "all",
			},
		},
	})
	c.Assert(override.Arch, jc.IsTrue)
	c.Assert(override.Series, jc.IsFalse)
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchArchAMD64Revision(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Revision: transport.InfoRevision{
			Platforms: []transport.Platform{{
				Architecture: "amd64",
			}, {
				Architecture: "arm64",
			}},
		},
	})
	c.Assert(override.Arch, jc.IsFalse)
	c.Assert(override.Series, jc.IsFalse)
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchArchAllRevision(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Revision: transport.InfoRevision{
			Platforms: []transport.Platform{{
				Architecture: "all",
			}},
		},
	})
	c.Assert(override.Arch, jc.IsTrue)
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchNoRevisions(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Revision: transport.InfoRevision{
			Platforms: []transport.Platform{},
		},
	})
	c.Assert(override.Arch, jc.IsFalse)
	c.Assert(override.Series, jc.IsFalse)
	c.Assert(matched, jc.IsFalse)
}

func (s channelPlatformSuite) TestMatchSeries(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/focal"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Series:       "focal",
			},
		},
	})
	c.Assert(override.Arch, jc.IsFalse)
	c.Assert(override.Series, jc.IsFalse)
	c.Assert(matched, jc.IsTrue)
}

func (s channelPlatformSuite) TestMatchSeriesAll(c *gc.C) {
	cp := channelPlatform{
		Platform: corecharm.MustParsePlatform("amd64/ubuntu/focal"),
	}

	override, matched := cp.MatchPlatform(transport.InfoChannelMap{
		Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: "amd64",
				OS:           "ubuntu",
				Series:       "all",
			},
		},
	})
	c.Assert(override.Arch, jc.IsFalse)
	c.Assert(override.Series, jc.IsTrue)
	c.Assert(matched, jc.IsTrue)
}
