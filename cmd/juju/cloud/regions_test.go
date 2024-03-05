// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"os"
	"strings"

	"github.com/juju/cmd/v4"
	"github.com/juju/cmd/v4/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type regionsSuite struct {
	testing.FakeJujuXDGDataHomeSuite

	api   *fakeShowCloudAPI
	store *jujuclient.MemStore
}

var _ = gc.Suite(&regionsSuite{})

func (s *regionsSuite) SetUpTest(c *gc.C) {
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

func (s *regionsSuite) TestListRegionsInvalidCloud(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "invalid", "--client")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)
	c.Assert(cmdtesting.Stderr(ctx), jc.Contains, "ERROR cloud invalid not found")
}

func (s *regionsSuite) TestListRegionsInvalidArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws", "another")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["another"\]`)
}

func (s *regionsSuite) TestListRegionsLocalOnly(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nClient Cloud Regions\nlondon\nparis\n")
}

func (s *regionsSuite) setupControllerData(c *gc.C) cmd.Command {
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
		func() (cloud.CloudRegionsAPI, error) {
			return s.api, nil
		})
}

func (s *regionsSuite) TestListRegions(c *gc.C) {
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

func (s *regionsSuite) TestListRegionsControllerOnly(c *gc.C) {
	aCommand := s.setupControllerData(c)
	ctx, err := cmdtesting.RunCommand(c, aCommand, "kloud", "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nController Cloud Regions\nhive  \nmind  \n")
}

func (s *regionsSuite) TestListRegionsBuiltInCloud(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "localhost", "--client")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "\nClient Cloud Regions\nlocalhost\n")
}

func (s *regionsSuite) TestListRegionsYaml(c *gc.C) {
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

func (s *regionsSuite) TestListNoController(c *gc.C) {
	ctx := cmdtesting.Context(c)
	ctx.Stdin = strings.NewReader("n\ny\n")
	command := cloud.NewListRegionsCommand()
	err := cmdtesting.InitCommand(command, []string{"kloud"})
	c.Assert(err, jc.ErrorIsNil)
	err = command.Run(ctx)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, `

Client Cloud Regions
london
paris
`[1:])
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "")
}

func (s *regionsSuite) TestListRegionsJson(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--format", "json", "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "{\"client-cloud-regions\":{\"london\":{\"endpoint\":\"http://london/1.0\"},\"paris\":{\"endpoint\":\"http://paris/1.0\"}}}\n")
}
