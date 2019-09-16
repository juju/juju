// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
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
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *regionsSuite) TestListRegionsInvalidCloud(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "invalid", "--local")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)
	c.Assert(c.GetTestLog(), jc.Contains, "cloud invalid not found")
}

func (s *regionsSuite) TestListRegionsInvalidArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "aws", "another")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["another"\]`)
}

func (s *regionsSuite) TestListRegionsLocalOnly(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "london\nparis\n\n")
}

func (s *regionsSuite) TestListRegions(c *gc.C) {
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
	aCommand := cloud.NewListRegionsCommandForTest(
		s.store,
		func() (cloud.CloudRegionsAPI, error) {
			return s.api, nil
		})
	ctx, err := cmdtesting.RunCommand(c, aCommand, "kloud", "--no-prompt", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
local-cloud-regions:
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

func (s *regionsSuite) TestListRegionsBuiltInCloud(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "localhost", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, "localhost\n\n")
}

func (s *regionsSuite) TestListRegionsYaml(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--format", "yaml", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, `
local-cloud-regions:
  london:
    endpoint: http://london/1.0
  paris:
    endpoint: http://paris/1.0
`[1:])
}

func (s *regionsSuite) TestListNoRegionsOnController(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "google")
	c.Assert(err, gc.DeepEquals, cmd.ErrSilent)
	c.Assert(c.GetTestLog(), jc.Contains, `Not listing regions for cloud "google" from a controller: no controller specified.`)
}

type regionDetails struct {
	Endpoint         string `json:"endpoint"`
	IdentityEndpoint string `json:"identity-endpoint"`
	StorageEndpoint  string `json:"storage-endpoint"`
}

func (s *regionsSuite) TestListRegionsJson(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListRegionsCommand(), "kloud", "--format", "json", "--local")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cmdtesting.Stdout(ctx), jc.DeepEquals, "{\"local-cloud-regions\":{\"london\":{\"endpoint\":\"http://london/1.0\"},\"paris\":{\"endpoint\":\"http://paris/1.0\"}}}\n")
}
