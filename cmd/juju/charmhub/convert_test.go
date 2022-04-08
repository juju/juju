// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/arch"
)

type filterSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&filterSuite{})

func (filterSuite) TestFilterChannels(c *gc.C) {
	tests := []struct {
		Name     string
		Arch     string
		Series   string
		Input    []transport.InfoChannelMap
		Expected map[string]Channel
	}{{
		Name:   "match all",
		Arch:   "all",
		Series: "all",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: arch.AllArches().StringList(),
				Series: []string{"bionic"},
			},
		},
	}, {
		Name:   "match all architectures",
		Arch:   "all",
		Series: "bionic",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: arch.AllArches().StringList(),
				Series: []string{"bionic"},
			},
		},
	}, {
		Name:   "match all series",
		Arch:   "amd64",
		Series: "all",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: []string{"amd64"},
				Series: []string{"bionic"},
			},
		},
	}, {
		Name:   "match only ppc64 with focal series",
		Arch:   "ppc64",
		Series: "focal",
		Input: []transport.InfoChannelMap{{
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}},
		Expected: map[string]Channel{},
	}, {
		Name:   "channel has all architectures with same series",
		Arch:   "amd64",
		Series: "bionic",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: arch.AllArches().StringList(),
				Series: []string{"bionic"},
			},
		},
	}, {
		Name:   "channel has all architectures with no matching series",
		Arch:   "amd64",
		Series: "focal",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{},
	}, {
		Name:   "multiple channels have all architectures with same series",
		Arch:   "amd64",
		Series: "focal",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "20.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: arch.AllArches().StringList(),
				Series: []string{"focal"},
			},
		},
	}, {
		Name:   "multiple channels have all architectures with no matching series",
		Arch:   "amd64",
		Series: "bionic",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "20.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: map[string]Channel{
			"latest/stable": {
				Track:  "latest",
				Risk:   "stable",
				Arches: []string{"amd64"},
				Series: []string{"bionic"},
			},
		},
	}, {
		Name:   "exact match finds no valid channels",
		Arch:   "ppc64",
		Series: "focal",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "arm64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Channel:      "18.04",
					Architecture: "ppc64",
				}},
			},
		}},
		Expected: map[string]Channel{},
	}, {
		Name:   "exact match finds no valid channels",
		Arch:   "amd64",
		Series: "focal",
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:       "xena/edge",
				Base:       transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
				ReleasedAt: "2022-04-01T02:41:31.140463+00:00",
				Risk:       "edge",
				Track:      "xena",
			},
			Revision: transport.InfoRevision{
				Bases:    []transport.Base{{Channel: "20.04", Name: "ubuntu", Architecture: "amd64"}},
				Revision: 522,
			},
		}, { // the 16.04 channel is intentionally the last one in the map
			Channel: transport.Channel{
				Name:       "xena/edge",
				Base:       transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "16.04"},
				ReleasedAt: "2022-03-04T10:38:13.959649+00:00",
				Risk:       "edge",
				Track:      "xena",
			},
			Revision: transport.InfoRevision{
				Bases:    []transport.Base{{Channel: "16.04", Name: "ubuntu", Architecture: "all"}},
				Revision: 501,
			},
		}},
		Expected: map[string]Channel{
			"xena/edge": {
				ReleasedAt: "2022-04-01T02:41:31.140463+00:00",
				Risk:       "edge",
				Track:      "xena",
				Revision:   522,
				Arches:     []string{"amd64"},
				Series:     []string{"focal"},
			},
		},
	}}
	for k, v := range tests {
		c.Logf("Test %d %s", k, v.Name)
		_, got := filterChannels(v.Input, false, v.Arch, v.Series)
		c.Assert(got, jc.DeepEquals, v.Expected)
	}
}
