// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"github.com/juju/tc"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/cmd/juju/charmhub/mocks"
	"github.com/juju/juju/core/arch"
	"github.com/juju/juju/internal/charmhub"
	"github.com/juju/juju/internal/charmhub/transport"
	"github.com/juju/juju/internal/cmd/cmdtesting"
)

type findSuite struct {
	testing.IsolationSuite

	charmHubAPI *mocks.MockCharmHubClient
}

var _ = tc.Suite(&findSuite{})

func (s *findSuite) TestInitNoArgs(c *tc.C) {
	// You can query the find api with no arguments.
	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		columns:         "nbvps",
	}
	err := command.Init([]string{})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *findSuite) TestInitSuccess(c *tc.C) {
	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
		columns:         "nbvps",
	}
	err := command.Init([]string{"test"})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *findSuite) TestRun(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *findSuite) TestRunJSON(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "json"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(indentJSON(c, cmdtesting.Stdout(ctx)), tc.Equals, `
[
  {
    "type": "object",
    "id": "charmCHARMcharmCHARMcharmCHARM01",
    "name": "wordpress",
    "publisher": "WordPress Charmers",
    "summary": "WordPress is a full featured web blogging tool, this charm deploys it.",
    "version": "1.0.3",
    "architectures": [
      "all"
    ],
    "os": [
      "ubuntu"
    ],
    "supports": [
      {
        "name": "ubuntu",
        "channel": "18.04"
      }
    ],
    "store-url": "https://someurl.com/wordpress"
  }
]
`[1:])
}

func (s *findSuite) TestRunYAML(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--format", "yaml"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `
- type: object
  id: charmCHARMcharmCHARMcharmCHARM01
  name: wordpress
  publisher: WordPress Charmers
  summary: WordPress is a full featured web blogging tool, this charm deploys it.
  version: 1.0.3
  architectures:
  - all
  os:
  - ubuntu
  supports:
  - name: ubuntu
    channel: "18.04"
  store-url: https://someurl.com/wordpress
`[1:])
}

func (s *findSuite) TestRunWithType(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--type", "bundle"})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *findSuite) TestRunWithNoType(c *tc.C) {
	defer s.setUpMocks(c).Finish()
	s.expectFind()

	command := &findCommand{
		charmHubCommand: s.newCharmHubCommand(),
	}

	err := cmdtesting.InitCommand(command, []string{"test", "--type", ""})
	c.Assert(err, tc.ErrorIsNil)

	ctx := commandContextForTest(c)
	err = command.Run(ctx)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *findSuite) newCharmHubCommand() *charmHubCommand {
	return &charmHubCommand{
		arches: arch.AllArches(),
		CharmHubClientFunc: func(charmhub.Config) (CharmHubClient, error) {
			return s.charmHubAPI, nil
		},
	}
}

func (s *findSuite) setUpMocks(c *tc.C) *gomock.Controller {
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
			Publisher: map[string]string{"display-name": "WordPress Charmers"},
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
