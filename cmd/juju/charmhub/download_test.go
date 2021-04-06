// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"context"
	"io"
	"net/url"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/arch"
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
	file               *mocks.MockReadSeekCloser
	filesystem         *mocks.MockFilesystem
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
	s.apiRoot.EXPECT().BestFacadeVersion("CharmHub").Return(1)

	url := "http://example.org/"

	s.expectModelGet(url)
	s.expectRefresh(url)
	s.expectDownload(c, url)
	s.expectFilesystem(c)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	command.SetClientStore(s.store)
	command.SetFilesystem(s.filesystem)
	cmd := modelcmd.Wrap(command, modelcmd.WrapSkipModelInit)
	err := cmdtesting.InitCommand(cmd, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = cmd.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.apiRoot.EXPECT().BestFacadeVersion("CharmHub").Return(1)

	url := "http://example.org/"

	s.expectModelGet(url)
	s.expectRefresh(url)
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

	s.expectRefresh(url)
	s.expectDownload(c, url)
	s.expectFilesystem(c)

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	command.SetFilesystem(s.filesystem)
	err := cmdtesting.InitCommand(command, []string{"--charmhub-url=" + url, "test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *downloadSuite) TestRunWithUnsupportedSeries(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.apiRoot.EXPECT().BestFacadeVersion("CharmHub").Return(1)

	url := "http://example.org/"

	s.expectModelGet(url)
	s.expectRefreshUnsupportedSeries()

	command := &downloadCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (DownloadCommandAPI, error) {
			return s.downloadCommandAPI, nil
		},
	}
	command.SetClientStore(s.store)
	command.SetFilesystem(s.filesystem)
	cmd := modelcmd.Wrap(command, modelcmd.WrapSkipModelInit)
	err := cmdtesting.InitCommand(cmd, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = cmd.Run(ctx)
	c.Assert(err, gc.ErrorMatches, "test does not support series focal in channel stable.  Supported series are bionic, trusty, xenial.")
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
	err := cmdtesting.InitCommand(command, []string{"--charmhub-url=" + url, "test"})
	c.Assert(err, gc.ErrorMatches, `unexpected charmhub-url: parse "meshuggah": invalid URI for request`)
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

	s.file = mocks.NewMockReadSeekCloser(ctrl)
	s.filesystem = mocks.NewMockFilesystem(ctrl)

	return ctrl
}

func (s *downloadSuite) expectModelGet(charmHubURL string) {
	s.modelConfigAPI.EXPECT().ModelGet().Return(map[string]interface{}{
		"type":         "my-type",
		"name":         "my-name",
		"uuid":         "deadbeef-0bad-400d-8000-4b1d0d06f00d",
		"charmhub-url": charmHubURL,
	}, nil)
}

func (s *downloadSuite) expectRefresh(charmHubURL string) {
	s.downloadCommandAPI.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
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
			},
		}}, nil
	})
}

func (s *downloadSuite) expectRefreshUnsupportedSeries() {
	s.downloadCommandAPI.EXPECT().Refresh(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, cfg charmhub.RefreshConfig) ([]transport.RefreshResponse, error) {
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
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "bionic"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "trusty"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "trusty"},
							Channel: "candidate",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "xenial"},
							Channel: "stable",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "focal"},
							Channel: "beta",
						},
						{
							Base:    transport.Base{Architecture: "amd64", Name: "ubuntu", Channel: "xenial"},
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
	s.downloadCommandAPI.EXPECT().Download(gomock.Any(), resourceURL, "test_e3b0c44.charm", gomock.Any()).Return(nil)
}

func (s *downloadSuite) expectFilesystem(c *gc.C) {
	s.file.EXPECT().Read(gomock.Any()).Return(0, io.EOF).AnyTimes()
	s.file.EXPECT().Close().Return(nil)
	s.filesystem.EXPECT().Open("test_e3b0c44.charm").Return(s.file, nil)
}
