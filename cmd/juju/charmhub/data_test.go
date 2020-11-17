// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/charmhub"
)

type filterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&filterSuite{})

func (filterSuite) TestConvertChannels(c *gc.C) {
	tests := []struct {
		Name     string
		Arch     string
		Series   string
		Input    map[string]charmhub.Channel
		Expected map[string]charmhub.Channel
	}{{
		Name:   "match all",
		Arch:   "all",
		Series: "all",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
	}, {
		Name:   "match all architectures",
		Arch:   "all",
		Series: "bionic",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
	}, {
		Name:   "match all series",
		Arch:   "amd64",
		Series: "all",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
	}, {
		Name:   "match only ppc64 with focal series",
		Arch:   "ppc64",
		Series: "focal",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{},
	}, {
		Name:   "channel has all architectures with same series",
		Arch:   "amd64",
		Series: "bionic",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
	}, {
		Name:   "channel has all architectures with no matching series",
		Arch:   "amd64",
		Series: "focal",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{},
	}, {
		Name:   "multiple channels have all architectures with same series",
		Arch:   "amd64",
		Series: "focal",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "focal",
				}},
			},
			"latest/edge": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "focal",
				}},
			},
		},
	}, {
		Name:   "multiple channels have all architectures with no matching series",
		Arch:   "amd64",
		Series: "bionic",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "all",
					Series:       "focal",
				}},
			},
			"latest/edge": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{
			"latest/edge": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "amd64",
					Series:       "bionic",
				}},
			},
		},
	}, {
		Name:   "exact match finds no valid channels",
		Arch:   "ppc64",
		Series: "focal",
		Input: map[string]charmhub.Channel{
			"latest/stable": charmhub.Channel{
				Platforms: []charmhub.Platform{{
					Architecture: "arm64",
					Series:       "bionic",
				}, {
					Architecture: "ppc64",
					Series:       "bionic",
				}},
			},
		},
		Expected: map[string]charmhub.Channel{},
	}}
	for k, v := range tests {
		c.Logf("Test %d %s", k, v.Name)
		got := filterChannels(v.Input, v.Arch, v.Series)
		c.Assert(got, jc.DeepEquals, v.Expected)
	}
}
