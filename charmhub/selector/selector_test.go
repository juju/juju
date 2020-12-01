// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package selector

import (
	"sort"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/core/charm"
)

type selectorSuite struct {
}

var _ = gc.Suite(&selectorSuite{})

func (s *selectorSuite) TestLocateRevisionByChannel(c *gc.C) {
	revisions, found := locateRevisionByChannel([]transport.InfoChannelMap{{
		Channel: transport.Channel{
			Name:  "a",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "xenial",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 1,
		},
	}, {
		Channel: transport.Channel{
			Name:  "b",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "bionic",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 2,
		},
	}, {
		Channel: transport.Channel{
			Name:  "c",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "focal",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 3,
		},
	}, {
		Channel: transport.Channel{
			Name:  "d",
			Track: "2.0",
			Risk:  "edge",
			Platform: transport.Platform{
				Series: "focal",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 4,
		},
	}}, charm.MustParseChannel("latest/stable"))

	c.Assert(found, jc.IsTrue)
	c.Assert(revisions, gc.HasLen, 3)
	c.Assert(revisions[0].Revision, gc.Equals, 1)
}

func (s *selectorSuite) TestLocateRevisionByChannelAfterSorted(c *gc.C) {
	selector := NewSelectorForDownload([]string{"focal", "bionic"})
	in := []transport.InfoChannelMap{{
		Channel: transport.Channel{
			Name:  "a",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "xenial",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 1,
		},
	}, {
		Channel: transport.Channel{
			Name:  "b",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "bionic",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 2,
		},
	}, {
		Channel: transport.Channel{
			Name:  "c",
			Track: "latest",
			Risk:  "stable",
			Platform: transport.Platform{
				Series: "focal",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 3,
		},
	}, {
		Channel: transport.Channel{
			Name:  "d",
			Track: "2.0",
			Risk:  "edge",
			Platform: transport.Platform{
				Series: "focal",
			},
		},
		Revision: transport.InfoRevision{
			Revision: 4,
		},
	}}
	in = selector.sortInfoChannelMap(in)
	revisions, found := locateRevisionByChannel(in, charm.MustParseChannel("latest/stable"))

	c.Assert(found, jc.IsTrue)
	c.Assert(revisions, gc.HasLen, 3)
	c.Assert(revisions[0].Revision, gc.Equals, 3)
}

func (s *selectorSuite) TestLocateRevisionByChannelMap(c *gc.C) {
	tests := []struct {
		InputTrack string
		InputRisk  string
		Channel    string
	}{{
		InputTrack: "",
		InputRisk:  "stable",
		Channel:    "latest/stable",
	}, {
		InputTrack: "latest",
		InputRisk:  "stable",
		Channel:    "latest/stable",
	}, {
		InputTrack: "2.0",
		InputRisk:  "",
		Channel:    "2.0/stable",
	}, {
		InputTrack: "",
		InputRisk:  "edge",
		Channel:    "edge",
	}}
	for i, test := range tests {
		c.Logf("test %d", i)

		revision, found := locateRevisionByChannelMap(transport.InfoChannelMap{
			Channel: transport.Channel{
				Track: test.InputTrack,
				Risk:  test.InputRisk,
			},
			Revision: transport.InfoRevision{
				Revision: 1,
			},
		}, charm.MustParseChannel(test.Channel))

		c.Assert(found, jc.IsTrue)
		c.Assert(revision.Revision, gc.Equals, 1)
	}
}

type matchingArchesSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&matchingArchesSuite{})

func (matchingArchesSuite) TestMatchingArch(c *gc.C) {
	revisions := []transport.InfoRevision{{
		Platforms: []transport.Platform{{
			Architecture: "all",
		}, {
			Architecture: "all",
		}},
	}, {
		Platforms: []transport.Platform{{
			Architecture: "all",
		}},
	}}
	match := listArchitectures(revisions)
	c.Assert(match, jc.DeepEquals, []string{"all"})
}

func (matchingArchesSuite) TestNonMatchingArch(c *gc.C) {
	revisions := []transport.InfoRevision{{
		Platforms: []transport.Platform{{
			Architecture: "all",
		}, {
			Architecture: "amd64",
		}},
	}, {
		Platforms: []transport.Platform{{
			Architecture: "all",
		}},
	}}
	match := listArchitectures(revisions)
	c.Assert(match, jc.DeepEquals, []string{"all", "amd64"})
}

type linkClosedChannelsSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&linkClosedChannelsSuite{})

func (linkClosedChannelsSuite) TestLinkClosedChannelsWithOneTrack(c *gc.C) {
	tests := []struct {
		Name   string
		In     []transport.InfoChannelMap
		Result []transport.InfoChannelMap
	}{{
		Name: "edge to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}},
	}, {
		Name: "edge then beta to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}},
	}, {
		Name: "candidate to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}},
	}, {
		Name: "edge then candidate to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "c",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 35,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "c",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}, {
			Channel: transport.Channel{
				Name:  "c",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 34,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 35,
			},
		}},
	}, {
		Name: "edge only",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 35,
			},
		}},
		Result: []transport.InfoChannelMap{
			{},
			{},
			{},
			{
				Channel: transport.Channel{
					Name:  "e",
					Track: "latest",
					Risk:  "edge",
				},
				Revision: transport.InfoRevision{
					Revision: 35,
				},
			},
		},
	}, {
		Name: "edge then candidate",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "c",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 35,
			},
		}, {
			Channel: transport.Channel{
				Name:  "e",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}},
		Result: []transport.InfoChannelMap{
			{},
			{
				Channel: transport.Channel{
					Name:  "c",
					Track: "latest",
					Risk:  "candidate",
				},
				Revision: transport.InfoRevision{
					Revision: 35,
				},
			},
			{
				Channel: transport.Channel{
					Name:  "c",
					Track: "latest",
					Risk:  "beta",
				},
				Revision: transport.InfoRevision{
					Revision: 35,
				},
			},
			{
				Channel: transport.Channel{
					Name:  "e",
					Track: "latest",
					Risk:  "edge",
				},
				Revision: transport.InfoRevision{
					Revision: 33,
				},
			},
		},
	}}

	for i, test := range tests {
		c.Logf("Running %d %s", i, test.Name)

		got, err := linkClosedChannels(test.In)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(got, jc.DeepEquals, test.Result)
	}
}

func (linkClosedChannelsSuite) TestLinkClosedChannelsWithMultipleTracks(c *gc.C) {
	tests := []struct {
		Name   string
		In     []transport.InfoChannelMap
		Result []transport.InfoChannelMap
	}{{
		Name: "edge to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}},
	}, {
		Name: "edge to stable",
		In: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "ez",
				Track: "1.0",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 134,
			},
		}},
		Result: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "a",
				Track: "latest",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 33,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "candidate",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "az",
				Track: "1.0",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Revision: 133,
			},
		}, {
			Channel: transport.Channel{
				Name:  "ez",
				Track: "1.0",
				Risk:  "edge",
			},
			Revision: transport.InfoRevision{
				Revision: 134,
			},
		}},
	}}

	for i, test := range tests {
		c.Logf("Running %d %s", i, test.Name)

		got, err := linkClosedChannels(test.In)
		c.Assert(err, jc.ErrorIsNil)

		sort.Slice(got, func(i, j int) bool {
			if got[i].Channel.Track == got[j].Channel.Track {
				return riskIndex(got[i]) < riskIndex(got[j])
			}
			return got[i].Channel.Track > got[j].Channel.Track
		})

		c.Assert(got, jc.DeepEquals, test.Result)
	}
}

type downloadFilterSuite struct {
	jujutesting.IsolationSuite
}

var _ = gc.Suite(&downloadFilterSuite{})

func (downloadFilterSuite) TestFilterByArchitecture(c *gc.C) {
	tests := []struct {
		Name   string
		In     string
		Value  string
		Result bool
	}{{
		Name:   "exact match",
		In:     "amd64",
		Value:  "amd64",
		Result: true,
	}, {
		Name:   "no match",
		In:     "amd64",
		Value:  "arm64",
		Result: false,
	}, {
		Name:   "any match",
		In:     "all",
		Value:  "all",
		Result: true,
	}, {
		Name:   "any filter match",
		In:     "all",
		Value:  "arm64",
		Result: true,
	}, {
		Name:   "any value match",
		In:     "amd64",
		Value:  "all",
		Result: true,
	}}

	for i, test := range tests {
		c.Logf("Running %d %s", i, test.Name)

		filter := NewSelectorForDownload(nil).filterByArchitecture(test.In)
		ok := filter(transport.InfoChannelMap{Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: test.Value,
			},
		}})
		c.Assert(ok, gc.Equals, test.Result)
	}
}

func (downloadFilterSuite) TestFilterBySeries(c *gc.C) {
	tests := []struct {
		Name   string
		In     string
		Value  string
		Result bool
	}{{
		Name:   "exact match",
		In:     "focal",
		Value:  "focal",
		Result: true,
	}, {
		Name:   "no match",
		In:     "focal",
		Value:  "bionic",
		Result: false,
	}, {
		Name:   "any match",
		In:     "all",
		Value:  "all",
		Result: true,
	}, {
		Name:   "any filter match",
		In:     "all",
		Value:  "bionic",
		Result: true,
	}, {
		Name:   "any value match",
		In:     "focal",
		Value:  "all",
		Result: true,
	}}

	for i, test := range tests {
		c.Logf("Running %d %s", i, test.Name)

		filter := NewSelectorForDownload(nil).filterBySeries(test.In)
		ok := filter(transport.InfoChannelMap{Channel: transport.Channel{
			Platform: transport.Platform{
				Series: test.Value,
			},
		}})
		c.Assert(ok, gc.Equals, test.Result)
	}
}

func (downloadFilterSuite) TestFilterByArchitectureAndSeries(c *gc.C) {
	tests := []struct {
		Name     string
		InArch   string
		InSeries string
		Arch     string
		Series   string
		Result   bool
	}{{
		Name:     "exact match",
		InArch:   "amd64",
		Arch:     "amd64",
		InSeries: "focal",
		Series:   "focal",
		Result:   true,
	}, {
		Name:     "no match",
		InArch:   "amd64",
		Arch:     "arm64",
		InSeries: "focal",
		Series:   "bionic",
		Result:   false,
	}, {
		Name:     "no arch match",
		InArch:   "amd64",
		Arch:     "arm64",
		InSeries: "focal",
		Series:   "focal",
		Result:   false,
	}, {
		Name:     "no series match",
		InArch:   "amd64",
		Arch:     "amd64",
		InSeries: "focal",
		Series:   "bionic",
		Result:   false,
	}, {
		Name:     "any match",
		InArch:   "all",
		Arch:     "all",
		InSeries: "all",
		Series:   "all",
		Result:   true,
	}, {
		Name:     "any filter match",
		InArch:   "all",
		Arch:     "amd64",
		InSeries: "all",
		Series:   "focal",
		Result:   true,
	}, {
		Name:     "any arch filter match",
		InArch:   "all",
		Arch:     "amd64",
		InSeries: "focal",
		Series:   "focal",
		Result:   true,
	}, {
		Name:     "any series filter match",
		InArch:   "amd64",
		Arch:     "amd64",
		InSeries: "all",
		Series:   "focal",
		Result:   true,
	}, {
		Name:     "any arch value match",
		InArch:   "amd64",
		Arch:     "all",
		InSeries: "focal",
		Series:   "focal",
		Result:   true,
	}, {
		Name:     "any series value match",
		InArch:   "amd64",
		Arch:     "amd64",
		InSeries: "focal",
		Series:   "all",
		Result:   true,
	}}

	for i, test := range tests {
		c.Logf("Running %d %s", i, test.Name)

		filter := NewSelectorForDownload(nil).filterByArchitectureAndSeries(test.InArch, test.InSeries)
		ok := filter(transport.InfoChannelMap{Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: test.Arch,
				Series:       test.Series,
			},
		}})
		c.Assert(ok, gc.Equals, test.Result)
	}
}
