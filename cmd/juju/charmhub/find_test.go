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
)

type findSuite struct {
	testing.IsolationSuite

	findCommandAPI *mocks.MockFindCommandAPI
	apiRoot        *basemocks.MockAPICallCloser
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) TestInitNoArgs(c *gc.C) {
	// You can query the find api with no arguments.
	command := &findCommand{
		columns: "nbvps",
	}
	err := command.Init([]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestInitSuccess(c *gc.C) {
	command := &findCommand{
		columns: "nbvps",
	}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
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
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
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
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
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

func (s *findSuite) TestRunWithType(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--type", "bundle"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRunWithNoType(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		APIRootFunc: func() (base.APICallCloser, error) {
			return s.apiRoot, nil
		},
		CharmHubClientFunc: func(api base.APICallCloser) FindCommandAPI {
			return s.findCommandAPI
		},
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--type", ""})
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
	}
}

func (s *findSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.findCommandAPI = mocks.NewMockFindCommandAPI(ctrl)
	s.findCommandAPI.EXPECT().Close().AnyTimes()

	s.apiRoot = basemocks.NewMockAPICallCloser(ctrl)
	s.apiRoot.EXPECT().Close().AnyTimes()
	s.apiRoot.EXPECT().BestFacadeVersion("CharmHub").Return(1)

	return ctrl
}

func (s *findSuite) expectFind() {
	s.findCommandAPI.EXPECT().Find("test", gomock.Any()).Return([]charmhub.FindResponse{{
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
