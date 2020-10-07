// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"net/url"

	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/testing"
)

type downloadSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	store *jujuclient.MemStore

	charmHubClient *mocks.MockCharmHubClient
	modelConfigAPI *mocks.MockModelConfigGetter
}

var _ = gc.Suite(&downloadSuite{})

func (s *downloadSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.store = jujuclienttesting.MinimalStore()
}

func (s *downloadSuite) TestInitNoArgs(c *gc.C) {
	command := &downloadCommand{}
	err := command.Init([]string{})
	c.Assert(err, gc.ErrorMatches, "expected a charm or bundle name")
}

func (s *downloadSuite) TestInitSuccess(c *gc.C) {
	command := &downloadCommand{}
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
		modelConfigAPI: s.modelConfigAPI,
		charmHubClient: s.charmHubClient,
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
		modelConfigAPI: s.modelConfigAPI,
		charmHubClient: s.charmHubClient,
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
		modelConfigAPI: s.modelConfigAPI,
		charmHubClient: s.charmHubClient,
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
		modelConfigAPI: s.modelConfigAPI,
		charmHubClient: s.charmHubClient,
	}
	err := cmdtesting.InitCommand(command, []string{"--charm-hub-url=" + url, "test"})
	c.Assert(err, gc.ErrorMatches, `unexpected charm-hub-url: parse "meshuggah": invalid URI for request`)
}

func (s *downloadSuite) TestRunWithInvalidStdout(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	command := &downloadCommand{
		modelConfigAPI: s.modelConfigAPI,
		charmHubClient: s.charmHubClient,
	}
	err := cmdtesting.InitCommand(command, []string{"test", "_"})
	c.Assert(err, gc.ErrorMatches, `expected a charm or bundle name, followed by hyphen to pipe to stdout`)
}

func (s *downloadSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmHubClient = mocks.NewMockCharmHubClient(ctrl)
	s.modelConfigAPI = mocks.NewMockModelConfigGetter(ctrl)
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
	s.charmHubClient.EXPECT().Info(gomock.Any(), "test").Return(transport.InfoResponse{
		Type: "charm",
		Name: "test",
		DefaultRelease: transport.ChannelMap{
			Revision: transport.Revision{
				Download: transport.Download{
					URL: charmHubURL,
				},
			},
		},
	}, nil)
}

func (s *downloadSuite) expectDownload(c *gc.C, charmHubURL string) {
	resourceURL, err := url.Parse(charmHubURL)
	c.Assert(err, jc.ErrorIsNil)
	s.charmHubClient.EXPECT().Download(gomock.Any(), resourceURL, "test.charm").Return(nil)
}
