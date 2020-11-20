// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"net/url"
	"sort"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	downloadCommandAPI *mocks.MockDownloadCommandAPI
	modelConfigAPI     *mocks.MockModelConfigClient
	apiRoot            *basemocks.MockAPICallCloser
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.MinimalStore()
}

func (s *downloadSuite) TestInitNoArgs(c *gc.C) {
	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := command.Init([]string{})
	c.Assert(err, gc.ErrorMatches, "expected a charm or bundle name")
}

func (s *downloadSuite) TestInitSuccess(c *gc.C) {
	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectModelGet(url)
	s.expectInfo(url)
	s.expectDownload(c, url)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	command.SetClientStore(s.store)
	cmd := modelcmd.Wrap(command, modelcmd.WrapSkipModelInit)
	err := cmdtesting.InitCommand(cmd, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = cmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectModelGet(url)
	s.expectInfo(url)
	s.expectDownload(c, url)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	command.SetClientStore(s.store)
	cmd := modelcmd.Wrap(command, modelcmd.WrapSkipModelInit)
	err := cmdtesting.InitCommand(cmd, []string{"test", "-"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = cmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithCustomCharmHubURL(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectInfo(url)
	s.expectDownload(c, url)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	err := cmdtesting.InitCommand(command, []string{"--charm-hub-url=" + url, "test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithCustomInvalidCharmHubURL(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "meshuggah"

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	err := cmdtesting.InitCommand(command, []string{"--charm-hub-url=" + url, "test"})
	c.Assert(err, gc.ErrorMatches, `unexpected charm-hub-url: parse "meshuggah": invalid URI for request`)
}

func (s *downloadSuite) TestRunWithInvalidStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	err := cmdtesting.InitCommand(command, []string{"test", "_"})
	c.Assert(err, gc.ErrorMatches, `expected a charm or bundle name, followed by hyphen to pipe to stdout`)
}

func (s *downloadSuite) newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
		ModelConfigClientFunc: func(api base.APICallCloser) ModelConfigClient {
			return s.modelConfigAPI
		},
	}
}

func (s *downloadSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.downloadCommandAPI = mocks.NewMockDownloadCommandAPI(ctrl)

	s.modelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	s.modelConfigAPI.EXPECT().Close().AnyTimes()

	s.apiRoot = basemocks.NewMockAPICallCloser(ctrl)
	s.apiRoot.EXPECT().Close().AnyTimes()

	return ctrl
}

func (s *downloadSuite) expectModelGet(charmHubURL string) {
	s.modelConfigAPI.EXPECT().ModelGet().Return(map[string]interface{}{
		"type":          "my-type",
		"name":          "my-name",
		"uuid":          "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"charm-hub-url": charmHubURL,
	}, nil)
}

func (s *downloadSuite) expectInfo(charmHubURL string) {
	s.downloadCommandAPI.EXPECT().Info(gomock.Any(), "test").Return(transport.InfoResponse{
		Type: "charm",
		Name: "test",
		ChannelMap: []transport.InfoChannelMap{{
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
				Download: transport.Download{
					URL: charmHubURL,
				},
			},
		}},
	}, nil)
}

func (s *downloadSuite) expectDownload(c *gc.C, charmHubURL string) {
	resourceURL, err := url.Parse(charmHubURL)
	c.Assert(err, jc.ErrorIsNil)
	s.downloadCommandAPI.EXPECT().Download(gomock.Any(), resourceURL, "test.charm").Return(nil)
}

func (s *downloadSuite) TestLocateRevisionByChannel(c *gc.C) {
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
	}}, corecharm.MustParseChannel("latest/stable"))

	c.Assert(found, jc.IsTrue)
	c.Assert(revisions, gc.HasLen, 3)
	c.Assert(revisions[0].Revision, gc.Equals, 1)
}

func (s *downloadSuite) TestLocateRevisionByChannelAfterSorted(c *gc.C) {
	command := &downloadCommand{
		orderedSeries: []string{"focal", "bionic"},
	}
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
	in = command.sortInfoChannelMap(in)
	revisions, found := locateRevisionByChannel(in, corecharm.MustParseChannel("latest/stable"))

	c.Assert(found, jc.IsTrue)
	c.Assert(revisions, gc.HasLen, 3)
	c.Assert(revisions[0].Revision, gc.Equals, 3)
}

func (s *downloadSuite) TestLocateRevisionByChannelMap(c *gc.C) {
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
		}, corecharm.MustParseChannel(test.Channel))

		c.Assert(found, jc.IsTrue)
		c.Assert(revision.Revision, gc.Equals, 1)
	}
}

func (s *downloadSuite) TestChannelMapSort(c *gc.C) {
	series := channelMapBySeries{
		channelMap: []transport.InfoChannelMap{{
			Channel: transport.Channel{
				Name: "a",
				Platform: transport.Platform{
					Series: "xenial",
				},
			},
		}, {
			Channel: transport.Channel{
				Name: "b",
				Platform: transport.Platform{
					Series: "bionic",
				},
			},
		}, {
			Channel: transport.Channel{
				Name: "c",
				Platform: transport.Platform{
					Series: "focal",
				},
			},
		}},
		series: []string{"focal", "bionic"},
	}

	sort.Sort(series)

	names := make([]string, 3)
	for k, v := range series.channelMap {
		names[k] = v.Channel.Name
	}

	c.Assert(names, gc.DeepEquals, []string{"c", "b", "a"})
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

		filter := filterByArchitecture(test.In)
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

		filter := filterBySeries(test.In)
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

		filter := filterByArchitectureAndSeries(test.InArch, test.InSeries)
		ok := filter(transport.InfoChannelMap{Channel: transport.Channel{
			Platform: transport.Platform{
				Architecture: test.Arch,
				Series:       test.Series,
			},
		}})
		c.Assert(ok, gc.Equals, test.Result)
	}
}
