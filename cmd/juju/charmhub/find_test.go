// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/cmd/v3/cmdtesting"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/charmhub"
	"github.com/juju/juju/charmhub/transport"
	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
)

type findSuite struct {
	testing.IsolationSuite

	charmHubAPI *mocks.MockCharmHubClient
}

var _ = gc.Suite(&findSuite{})

func (s *findSuite) TestInitNoArgs(c *gc.C) {
	// You can query the find api with no arguments.
	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		columns:         "nbvps",
	}
	err := command.Init([]string{})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestInitSuccess(c *gc.C) {
	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		columns:         "nbvps",
	}
	err := command.Init([]string{"test"})
	c.Assert(err, jc.ErrorIsNil)
}

func (s *findSuite) TestRun(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
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
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json"})
	c.Assert(err, jc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `[{"type":"object","id":"charmCHARMcharmCHARMcharmCHARM01","name":"wordpress","publisher":"Wordress Charmers","summary":"WordPress is a full featured web blogging tool, this charm deploys it.","version":"1.0.3","architectures":["all"],"os":["ubuntu"],"series":["bionic"],"store-url":"https://someurl.com/wordpress"}]
`)
}

func (s *findSuite) TestRunYAML(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
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
  os:
  - ubuntu
  series:
  - bionic
  store-url: https://someurl.com/wordpress
`[1:])
}

func (s *findSuite) TestRunWithType(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
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
		charmHubCommand: s.newCharmHubCommand(),
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
		CharmHubClientFunc: func(charmhub.Config, charmhub.FileSystem) (CharmHubClient, error) {
			return s.charmHubAPI, nil
		},
	}
}

func (s *findSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmHubAPI = mocks.NewMockCharmHubClient(ctrl)
	return ctrl
}

func (s *findSuite) expectFind() {
	s.charmHubAPI.EXPECT().Find(gomock.Any(), "test", gomock.Any()).Return([]transport.FindResponse{{
		Name: "wordpress",
		Type: "object",
		ID:   "charmCHARMcharmCHARMcharmCHARM01",
		Entity: transport.Entity{
			Publisher: map[string]string{"display-name": "Wordress Charmers"},
			Summary:   "WordPress is a full featured web blogging tool, this charm deploys it.",
			StoreURL:  "https://someurl.com/wordpress",
		},
		DefaultRelease: transport.FindChannelMap{
			Revision: transport.FindRevision{
				Version: "1.0.3",
				Bases: []transport.Base{{
					Name:         "ubuntu",
					Channel:      "18.04",
					Architecture: "all",
				}},
			},
		},
	}}, nil)
}
