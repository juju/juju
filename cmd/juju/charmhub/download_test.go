// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/url"

	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	charmHubAPI *mocks.MockCharmHubClient
	file        *mocks.MockReadSeekCloser
	filesystem  *mocks.MockFilesystem
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

func (s *downloadSuite) TestInitErrorCSSchema(c *gc.C) {
	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := command.Init([]string{"cs:test"})
	c.Assert(errors.Is(err, errors.NotValid), jc.IsTrue)
}

func (s *downloadSuite) TestInitSuccess(c *gc.C) {
	s.testInitSuccess(c, "test")
}

func (s *downloadSuite) TestInitSuccessWithSchema(c *gc.C) {
	s.testInitSuccess(c, "ch:test")
}

func (s *downloadSuite) testInitSuccess(c *gc.C, charmName string) {
	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := command.Init([]string{charmName})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectRefresh(url)
	s.expectDownload(c, url)
	s.expectFilesystem(c)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stderr(ctx), gc.Matches, "(?s)"+`
Fetching charm "test" revision 123 using "stable" channel and base "amd64/ubuntu/22.04"
Install the "test" charm with:
    juju deploy ./test_r123\.charm
`[1:])
}

func (s *downloadSuite) TestRunWithStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectRefresh(url)
	s.expectDownload(c, url)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := cmdtesting.InitCommand(command, []string{"test", "-"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithCustomCharmHubURL(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectRefresh(url)
	s.expectDownload(c, url)
	s.expectFilesystem(c)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"--charmhub-url=" + url, "test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithUnsupportedSeriesPicksFirstSuggestion(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "http://example.org/"

	s.expectRefreshUnsupportedBase()
	s.expectRefresh(url)
	s.expectDownload(c, url)
	s.expectFilesystem(c)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithUnsupportedSeriesReturnsSecondAttempt(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.expectRefreshUnsupportedBase()
	s.expectRefreshUnsupportedBase()

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, `"test" does not support base ".*" in channel "stable"\. Supported bases are: ubuntu@18\.04, ubuntu@14\.04, ubuntu@16\.04\.`)
}

func (s *downloadSuite) TestRunWithNoStableRelease(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	s.expectRefreshUnsupportedBase()

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"test", "--channel", "foo/stable"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, gc.ErrorMatches, `"test" has no releases in channel "foo/stable". Type
    juju info test
for a list of supported channels.`)
}

func (s *downloadSuite) TestRunWithCustomInvalidCharmHubURL(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	url := "meshuggah"

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := cmdtesting.InitCommand(command, []string{"--charmhub-url=" + url, "test"})
	c.Assert(err, gc.ErrorMatches, `invalid charmhub-url: parse "meshuggah": invalid URI for request`)
}

func (s *downloadSuite) TestRunWithInvalidStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}
	err := cmdtesting.InitCommand(command, []string{"test", "_"})
	c.Assert(err, gc.ErrorMatches, `expected a charm or bundle name, followed by hyphen to pipe to stdout`)
}

func (s *downloadSuite) newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		CharmHubClientFunc: func(charmhub.Config) (CharmHubClient, error) {
			return s.charmHubAPI, nil
		},
	}
}

func (s *downloadSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.charmHubAPI = mocks.NewMockCharmHubClient(ctrl)

	s.file = mocks.NewMockReadSeekCloser(ctrl)
	s.filesystem = mocks.NewMockFilesystem(ctrl)

	return ctrl
}

func (s *downloadSuite) expectRefresh(charmHubURL string) {
	s.charmHubAPI.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			InstanceKey: instanceKey,
			Entity: transport.RefreshEntity{
				Type: transport.CharmType,
				Name: "test",
				Download: transport.Download{
					HashSHA256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
					URL:        charmHubURL,
				},
				Revision: 123,
			},
		}}, nil
	})
}

func (s *downloadSuite) expectRefreshUnsupportedBase() {
	s.charmHubAPI.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
		instanceKey := charmhub.ExtractConfigInstanceKey(cfg)

		return []transport.RefreshResponse{{
			InstanceKey: instanceKey,
			Entity: transport.RefreshEntity{
				Type: transport.CharmType,
				Name: "test",
			},
			Error: &transport.APIError{
				Code:    "revision-not-found",
				Message: "No revision was found in the Store.",
				Extra: transport.APIErrorExtra{
					Releases: []transport.Release{
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "18.04"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "14.04"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "14.04"},
							Channel: "candidate",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "16.04"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "20.04"},
							Channel: "beta",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "14.04"},
							Channel: "edge",
						},
					},
					DefaultBases: nil,
				}},
		}}, nil
	})
}

func (s *downloadSuite) expectDownload(c *gc.C, charmHubURL string) {
	resourceURL, err := url.Parse(charmHubURL)
	c.Assert(err, jc.ErrorIsNil)
	s.charmHubAPI.EXPECT().Download(gomock.Any(), resourceURL, "test_r123.charm", gomock.Any()).Return(nil)
}

func (s *downloadSuite) expectFilesystem(c *gc.C) {
	s.file.EXPECT().Read(gomock.Any()).Return(0, io.EOF).AnyTimes()
	s.file.EXPECT().Close().Return(nil)
	s.filesystem.EXPECT().Open("test_r123.charm").Return(s.file, nil)
}
