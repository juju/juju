// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type listSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeListCloudsAPI
	store *jujuclient.MemStore
}

var _ = gc.Suite(&listSuite{})

func (s *listSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeListCloudsAPI{}
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *listSuite) TestListPublic(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil), "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)

	// Check that we are producing the expected fields
	c.Assert(out, gc.Matches, `Cloud +Regions +Default +Type +Description.*`)
	// // Just check couple of snippets of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `.*aws +[0-9]+ +[a-z0-9-]+ +ec2 +Amazon Web Services.*`)
	c.Assert(out, gc.Matches, `.*azure +[0-9]+ +[a-z0-9-]+ +azure +Microsoft Azure.*`)
	// LXD should be there too.
	c.Assert(out, gc.Matches, `.*localhost[ ]*1[ ]*localhost[ ]*lxd.*`)
}

func (s *listSuite) TestListPublicLocalDefault(c *gc.C) {
	s.store.Controllers = nil
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil))
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `There are no controllers running.You can bootstrap a new controller using one of these clouds:.*`)

	// Check that we are producing the expected fields
	out = cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `.*Cloud +Regions +Default +Type +Description.*`)
	// // Just check couple of snippets of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `.*aws +[0-9]+ +[a-z0-9-]+ +ec2 +Amazon Web Services.*`)
	c.Assert(out, gc.Matches, `.*azure +[0-9]+ +[a-z0-9-]+ +azure +Microsoft Azure.*`)
	// LXD should be there too.
	c.Assert(out, gc.Matches, `.*localhost[ ]*1[ ]*localhost[ ]*lxd.*`)
}

func (s *listSuite) TestListController(c *gc.C) {
	var controllerAPICalled string
	cmd := cloud.NewListCloudCommandForTest(
		s.store,
		func(controllerName string) (cloud.ListCloudsAPI, error) {
			controllerAPICalled = controllerName
			return s.api, nil
		})
	s.api.controllerClouds = make(map[names.CloudTag]jujucloud.Cloud)
	s.api.controllerClouds[names.NewCloudTag("beehive")] = jujucloud.Cloud{
		Name:      "beehive",
		Type:      "openstack",
		AuthTypes: []jujucloud.AuthType{"userpass", "access-key"},
		Endpoint:  "http://myopenstack",
		Regions: []jujucloud.Region{
			{
				Name:     "regionone",
				Endpoint: "http://boston/1.0",
			},
		},
	}

	ctx, err := cmdtesting.RunCommand(c, cmd, "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Clouds", "Close")
	c.Assert(controllerAPICalled, gc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)

	// Just check couple of snippets of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `^beehive:.*type:\ openstack.*`)
}

func (s *listSuite) TestListKubernetes(c *gc.C) {
	var controllerAPICalled string
	cmd := cloud.NewListCloudCommandForTest(
		s.store,
		func(controllerName string) (cloud.ListCloudsAPI, error) {
			controllerAPICalled = controllerName
			return s.api, nil
		})
	s.api.controllerClouds = make(map[names.CloudTag]jujucloud.Cloud)
	s.api.controllerClouds[names.NewCloudTag("beehive")] = jujucloud.Cloud{
		Name:      "beehive",
		Type:      "kubernetes",
		AuthTypes: []jujucloud.AuthType{"userpass"},
		Endpoint:  "http://cluster",
		Regions: []jujucloud.Region{
			{
				Name:     "default",
				Endpoint: "http://cluster/default",
			},
		},
	}

	ctx, err := cmdtesting.RunCommand(c, cmd, "--controller", "mycontroller", "--format", "yaml")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Clouds", "Close")
	c.Assert(controllerAPICalled, gc.Equals, "mycontroller")
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)

	// Just check couple of snippets of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `^beehive:.*type:\ k8s.*`)
}

func (s *listSuite) assertListTabular(c *gc.C, expectedOutput string) {
	s.api.controllerClouds = make(map[names.CloudTag]jujucloud.Cloud)
	s.api.controllerClouds[names.NewCloudTag("beehive")] = jujucloud.Cloud{
		Name:      "beehive",
		Type:      "kubernetes",
		AuthTypes: []jujucloud.AuthType{"userpass"},
		Endpoint:  "http://cluster",
		Regions: []jujucloud.Region{
			{
				Name:     "default",
				Endpoint: "http://cluster/default",
			},
		},
	}
	s.api.controllerClouds[names.NewCloudTag("antnest")] = jujucloud.Cloud{
		Name:      "antnest",
		Type:      "openstack",
		AuthTypes: []jujucloud.AuthType{"userpass"},
		Endpoint:  "http://endpoint",
		Regions: []jujucloud.Region{
			{
				Name:     "default",
				Endpoint: "http://endpoint/default",
			},
		},
	}
	var controllerAPICalled string
	cmd := cloud.NewListCloudCommandForTest(
		s.store,
		func(controllerName string) (cloud.ListCloudsAPI, error) {
			controllerAPICalled = controllerName
			return s.api, nil
		})

	ctx, err := cmdtesting.RunCommand(c, cmd, "--controller", "mycontroller", "--format", "tabular")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "Clouds", "Close")
	c.Assert(controllerAPICalled, gc.Equals, "mycontroller")

	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Clouds on controller "mycontroller":.*`)

	out = cmdtesting.Stdout(ctx)
	c.Assert(out, jc.DeepEquals, expectedOutput)
}

func (s *listSuite) TestListTabular(c *gc.C) {
	s.assertListTabular(c, `
Cloud    Regions  Default  Type       Description
antnest        1  default  openstack  
beehive        1  default  k8s        

`[1:])
}

func (s *listSuite) TestListTabularWithClientDefaultRegion(c *gc.C) {
	s.store.Credentials["antnest"] = jujucloud.CloudCredential{DefaultRegion: "anotherregion"}
	s.assertListTabular(c, `
Cloud    Regions  Default        Type       Description
antnest        1  anotherregion  openstack  
beehive        1  default        k8s        

`[1:])
}

func (s *listSuite) TestListPublicAndPersonal(c *gc.C) {
	data := `
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	cmd := cloud.NewListCloudCommandForTest(
		s.store,
		func(controllerName string) (cloud.ListCloudsAPI, error) {
			c.Fail()
			return s.api, nil
		})

	ctx, err := cmdtesting.RunCommand(c, cmd, "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)
	// Just check a snippet of the output to make sure it looks ok.
	// local clouds are last.
	// homestack should abut localhost and hence come last in the output.
	c.Assert(out, jc.Contains, `homestack             1  london           openstack   Openstack Cloud`)
}

func (s *listSuite) TestListPublicAndPersonalSameName(c *gc.C) {
	data := `
clouds:
  aws:
    type: ec2
    auth-types: [access-key]
    endpoint: http://custom
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)
	c.Assert(err, jc.ErrorIsNil)

	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil), "--format", "yaml", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)
	// Just check a snippet of the output to make sure it looks ok.
	// local clouds are last.
	c.Assert(out, gc.Not(gc.Matches), `.*aws:[ ]*defined: public[ ]*type: ec2[ ]*auth-types: \[access-key\].*`)
	c.Assert(out, gc.Matches, `.*aws:[ ]*defined: local[ ]*type: ec2[ ]*description: Amazon Web Services[ ]*auth-types: \[access-key\].*`)
}

func (s *listSuite) TestListYAML(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil), "--format", "yaml", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)
	// Just check a snippet of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `.*aws:[ ]*defined: public[ ]*type: ec2[ ]*description: Amazon Web Services[ ]*auth-types: \[access-key\].*`)
}

func (s *listSuite) TestListJSON(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil), "--format", "json", "--local")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	out = strings.Replace(out, "\n", "", -1)
	// Just check a snippet of the output to make sure it looks ok.
	c.Assert(out, gc.Matches, `.*{"aws":{"defined":"public","type":"ec2","description":"Amazon Web Services","auth-types":\["access-key"\].*`)
}

func (s *listSuite) TestListPreservesRegionOrder(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewListCloudCommandForTest(s.store, nil), "--format", "yaml", "--local")
	c.Assert(err, jc.ErrorIsNil)
	lines := strings.Split(cmdtesting.Stdout(ctx), "\n")
	withClouds := "clouds:\n  " + strings.Join(lines, "\n  ")

	parsedClouds, err := jujucloud.ParseCloudMetadata([]byte(withClouds))
	c.Assert(err, jc.ErrorIsNil)
	parsedCloud, ok := parsedClouds["aws"]
	c.Assert(ok, jc.IsTrue) // aws found in output

	aws, err := jujucloud.CloudByName("aws")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&parsedCloud, jc.DeepEquals, aws)
}

type fakeListCloudsAPI struct {
	jujutesting.Stub
	controllerClouds map[names.CloudTag]jujucloud.Cloud
}

func (api *fakeListCloudsAPI) Close() error {
	api.AddCall("Close", nil)
	return nil
}

func (api *fakeListCloudsAPI) Clouds() (map[names.CloudTag]jujucloud.Cloud, error) {
	api.AddCall("Clouds")
	return api.controllerClouds, nil
}
