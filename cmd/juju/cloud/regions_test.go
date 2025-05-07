// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"context"
	"os"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
)

type regionsSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	api   *fakeShowCloudAPI
	store *jujuclient.MemStore
}

var _ = tc.Suite(&regionsSuite{})

func (s *regionsSuite) SetUpTest(c *tc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeShowCloudAPI{}
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store

	data := `
clouds:
  kloud:
    type: ec2
    auth-types: [access-key]
    endpoint: http://custom
    regions:
      london:
         endpoint: "http://london/1.0"
      paris:
         endpoint: "http://paris/1.0"
`[1:]
	err := os.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *regionsSuite) TestListRegionsInvalidCloud(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "invalid", "--client")
	c.Assert(err, tc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "ERROR cloud invalid not found")
}

func (s *regionsSuite) TestListRegionsInvalidArgs(c *tc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws", "another")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["another"\]`)
}

func (s *regionsSuite) TestListRegionsLocalOnly(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nClient Cloud Regions\nlondon\nparis\n")
}

func (s *regionsSuite) setupControllerData(c *tc.C) cmd.Command {
	s.api.cloud = jujucloud.Cloud{
		Name:        "beehive",
		Type:        "kubernetes",
		Description: "Bumble Bees",
		AuthTypes:   []jujucloud.AuthType{"userpass"},
		Endpoint:    "http://cluster",
		Regions: []jujucloud.Region{
			{
				Name:     "hive",
				Endpoint: "http://cluster/default",
			},
			{
				Name:     "mind",
				Endpoint: "http://cluster/default",
			},
		},
	}
	return cloud.NewListRegionsCommandForTest(
		s.store,
		func(ctx context.Context) (cloud.CloudRegionsAPI, error) {
			return s.api, nil
		})
}

func (s *regionsSuite) TestListRegions(c *tc.C) {
	aCommand := s.setupControllerData(c)
	ctx, err := cmdtesting.RunCommand(c, aCommand, "kloud", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
client-cloud-regions:
  london:
    endpoint: http://london/1.0
  paris:
    endpoint: http://paris/1.0
controller-cloud-regions:
  hive:
    endpoint: http://cluster/default
  mind:
    endpoint: http://cluster/default
`[1:])
}

func (s *regionsSuite) TestListRegionsControllerOnly(c *tc.C) {
	aCommand := s.setupControllerData(c)
	ctx, err := cmdtesting.RunCommand(c, aCommand, "kloud", "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nController Cloud Regions\nhive  \nmind  \n")
}

func (s *regionsSuite) TestListRegionsBuiltInCloud(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "localhost", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nClient Cloud Regions\nlocalhost\n")
}

func (s *regionsSuite) TestListRegionsYaml(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--format", "yaml", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
client-cloud-regions:
  london:
    endpoint: http://london/1.0
  paris:
    endpoint: http://paris/1.0
`[1:])
}

func (s *regionsSuite) TestListNoController(c *tc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("n\ny\n")
	command := cloud.NewListRegionsCommand()
	err := cmdtesting.InitCommand(command, []string{"kloud"})
	c.Assert(err, jc.ErrorIsNil)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), tc.Equals, `

Client Cloud Regions
london
paris
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), tc.Equals, "")
}

func (s *regionsSuite) TestListRegionsJson(c *tc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--format", "json", "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "{\"client-cloud-regions\":{\"london\":{\"endpoint\":\"http://london/1.0\"},\"paris\":{\"endpoint\":\"http://paris/1.0\"}}}\n")
}
