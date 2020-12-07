// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/charmhub"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/environs/config"
)

type findSuite struct {
	testing.IsolationSuite

	findCommandAPI *mocks.MockFindCommandAPI
	modelConfigAPI *mocks.MockModelConfigClient
	apiRoot        *basemocks.MockAPICallCloser
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) TestInitNoArgs(c *gc.C) {
	// You can query the find api with no arguments.
	command := &findCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
		columns: "nbvps",
	}
	err := command.Init([]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestInitSuccess(c *gc.C) {
	command := &findCommand{
		charmHubCommand: &charmHubCommand{
			arches: arch.AllArches(),
		},
		columns: "nbvps",
	}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRunJSON(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `[{"type":"object","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","version":"1.0.3","architectures":["all"],"series":["bionic"],"store-url":"https://someurl.com/wordpress"}]
`)
}

func (s *findSuite) TestRunYAML(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "yaml"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `
- type: object
  id: charmCHARMcharmCHARMcharmCHARM01
  name: wordpress
  publisher: Wordress Charmers
  summary: WordPress is a full featured web blogging tool, this charm deploys it.
  version: 1.0.3
  architectures:
  - all
  series:
  - bionic
  store-url: https://someurl.com/wordpress
`[1:])
}

func (s *findSuite) TestRunWithSeries(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", "bionic"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRunWithNoSeries(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()
	s.expectModelConfig(c, "bionic")

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--series", ""})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) newCharmHubCommand() *charmHubCommand {
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

func (s *findSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.findCommandAPI = mocks.NewMockFindCommandAPI(ctrl)
	s.findCommandAPI.EXPECT().Close().AnyTimes()

	s.modelConfigAPI = mocks.NewMockModelConfigClient(ctrl)
	s.modelConfigAPI.EXPECT().Close().AnyTimes()

	s.apiRoot = basemocks.NewMockAPICallCloser(ctrl)
	s.apiRoot.EXPECT().Close().AnyTimes()

	return ctrl
}

func (s *findSuite) expectFind() {
	s.findCommandAPI.EXPECT().Find("test").Return([]charmhub.FindResponse{{
		Name:      "wordpress",
		Type:      "object",
		ID:        "charmCHARMcharmCHARMcharmCHARM01",
		Publisher: "Wordress Charmers",
		Summary:   "WordPress is a full featured web blogging tool, this charm deploys it.",
		Version:   "1.0.3",
		Arches:    []string{"all"},
		Series:    []string{"bionic"},
		StoreURL:  "https://someurl.com/wordpress",
	}}, nil)
}

func (s *findSuite) expectModelConfig(c *gc.C, series string) {
	cfg, err := config.New(config.UseDefaults, map[string]interface{}{
		"default-series": series,
		"type":           "my-type",
		"name":           "my-name",
		"uuid":           "deadbeef-0bad-400d-8000-4b1d0d06f00d",
	})
	c.Assert(err, jc.ErrorIsNil)
	s.modelConfigAPI.EXPECT().ModelGet().Return(cfg.AllAttrs(), nil)
}
