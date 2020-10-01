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
)

type downloadSuite struct {
	charmHubClient *mocks.MockCharmHubClient
	modelConfigAPI *mocks.MockModelConfigGetter
}

var _ = gc.Suite(&downloadSuite{})

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
	cmdtesting.InitCommand(command, []string{"test"})
	ctx := commandContextForTest(c)
	err := command.Run(ctx)
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
	cmdtesting.InitCommand(command, []string{"--charm-hub-url=" + url, "test"})
	ctx := commandContextForTest(c)
	err := command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
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
	s.charmHubClient.EXPECT().Download(gomock.Any(), resourceURL, "").Return(nil)
}
