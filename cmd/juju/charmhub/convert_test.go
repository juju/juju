// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/testhelpers"
)

type filterSuite struct {
	testhelpers.IsolationSuite
}

func TestFilterSuite(t *stdtesting.T) { tc.Run(t, &filterSuite{}) }
func (s *filterSuite) TestFilterChannels(c *tc.C) {
	tests := []struct {
		Name     string
		Arch     string
		Risk     charm.Risk
		Revision int
		Track    string
		Base     corebase.Base
		Input    []transport.InfoChannelMap
		Expected RevisionsMap
	}{{
		Name:     "match all with no filters",
		Arch:     "all",
		Risk:     "",
		Revision: -1,
		Track:    "",
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name:     "filter by risk",
		Arch:     "all",
		Risk:     "edge",
		Revision: -1,
		Track:    "",
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "edge"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
				"edge": {{
					Track:  "latest",
					Risk:   "edge",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name:     "filter by revision",
		Arch:     "all",
		Risk:     "",
		Revision: 42,
		Track:    "",
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Revision: 42,
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "edge"},
			Revision: transport.InfoRevision{
				Revision: 43,
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:    "latest",
					Risk:     "stable",
					Revision: 42,
					Arches:   arch.AllArches().StringList(),
					Bases:    []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name:     "filter by track",
		Arch:     "all",
		Risk:     "",
		Revision: -1,
		Track:    "2.0",
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Track: "1.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}, {
			Channel: transport.Channel{
				Track: "2.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"2.0": {
				"stable": {{
					Track:  "2.0",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name:     "filter by channel configured with track and risk",
		Arch:     "all",
		Risk:     "beta",
		Revision: -1,
		Track:    "2.0",
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Track: "1.0",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}, {
			Channel: transport.Channel{
				Track: "2.0",
				Risk:  "stable",
			},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}, {
			Channel: transport.Channel{
				Track: "2.0",
				Risk:  "beta",
			},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"2.0": {
				"stable": {{
					Track:  "2.0",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
				"beta": {{
					Track:  "2.0",
					Risk:   "beta",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "match all",
		Arch: "all",
		Base: corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "match all architectures",
		Arch: "all",
		Base: corebase.MustParseBaseFromString("ubuntu@18.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "match all series",
		Arch: "amd64",
		Base: corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: []string{"amd64"},
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "match only ppc64 with focal series",
		Arch: "ppc64",
		Base: corebase.MustParseBaseFromString("ubuntu@20.04"),
		Input: []transport.InfoChannelMap{{
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}},
		Expected: RevisionsMap{},
	}, {
		Name: "channel has all architectures with same series",
		Arch: "amd64",
		Base: corebase.MustParseBaseFromString("ubuntu@18.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "channel has all architectures with no matching series",
		Arch: "amd64",
		Base: corebase.MustParseBaseFromString("ubuntu@20.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{},
	}, {
		Name: "multiple channels have all architectures with same series",
		Arch: "amd64",
		Base: corebase.MustParseBaseFromString("ubuntu@20.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "20.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: arch.AllArches().StringList(),
					Bases:  []Base{{Name: "ubuntu", Channel: "20.04"}},
				}},
			},
		},
	}, {
		Name: "multiple channels have all architectures with no matching series",
		Arch: "amd64",
		Base: corebase.MustParseBaseFromString("ubuntu@18.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "amd64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "20.04",
					Architecture: "all",
				}},
			},
		}},
		Expected: RevisionsMap{
			"latest": {
				"stable": {{
					Track:  "latest",
					Risk:   "stable",
					Arches: []string{"amd64"},
					Bases:  []Base{{Name: "ubuntu", Channel: "18.04"}},
				}},
			},
		},
	}, {
		Name: "exact match finds no valid channels",
		Arch: "ppc64",
		Base: corebase.MustParseBaseFromString("ubuntu@20.04"),
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "arm64",
				}},
			},
		}, {
			Channel: transport.Channel{Risk: "stable"},
			Revision: transport.InfoRevision{
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "ppc64",
				}},
			},
		}},
		Expected: RevisionsMap{},
	}, {
		Name:     "exact match finds no valid channels",
		Arch:     "amd64",
		Revision: -1,
		Base:     corebase.MustParseBaseFromString("ubuntu@20.04"),
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
		Expected: RevisionsMap{
			"xena": {
				"edge": {{
					ReleasedAt: "2022-04-01T02:41:31.140463+00:00",
					Risk:       "edge",
					Track:      "xena",
					Revision:   522,
					Arches:     []string{"amd64"},
					Bases:      []Base{{Name: "ubuntu", Channel: "20.04"}},
				}},
			},
		},
	}, {
		Name:     "sorts latest revisions first",
		Arch:     "all",
		Revision: -1,
		Base:     corebase.Base{},
		Input: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name:       "xena/edge",
				Base:       transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
				ReleasedAt: "2022-02-01T02:41:31.140463+00:00",
				Risk:       "edge",
				Track:      "xena",
			},
			Revision: transport.InfoRevision{
				Bases:    []transport.Base{{Channel: "20.04", Name: "ubuntu", Architecture: "amd64"}},
				Revision: 500,
			},
		}, {
			Channel: transport.Channel{
				Name:       "xena/edge",
				Base:       transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
				ReleasedAt: "2022-03-04T10:38:13.959649+00:00",
				Risk:       "edge",
				Track:      "xena",
			},
			Revision: transport.InfoRevision{
				Bases:    []transport.Base{{Channel: "20.04", Name: "ubuntu", Architecture: "amd64"}},
				Revision: 501,
			},
		}},
		Expected: RevisionsMap{
			"xena": {
				"edge": {{
					ReleasedAt: "2022-03-04T10:38:13.959649+00:00",
					Risk:       "edge",
					Track:      "xena",
					Revision:   501,
					Arches:     []string{"amd64"},
					Bases:      []Base{{Name: "ubuntu", Channel: "20.04"}},
				}, {
					ReleasedAt: "2022-02-01T02:41:31.140463+00:00",
					Risk:       "edge",
					Track:      "xena",
					Revision:   500,
					Arches:     []string{"amd64"},
					Bases:      []Base{{Name: "ubuntu", Channel: "20.04"}},
				}},
			},
		},
	}}
	for k, v := range tests {
		c.Logf("Test %d %s", k, v.Name)
		_, got, err := filterChannels(v.Input, v.Arch, v.Risk, v.Revision, v.Track, v.Base)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(got, tc.DeepEquals, v.Expected)
	}
}
