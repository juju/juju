// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	"github.com/juju/errors"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type updateCloudSuite struct {
	testing.FakeJujuXDGDataHomeSuite
	api   *fakeUpdateCloudAPI
	store *jujuclient.MemStore
}

var _ = gc.Suite(&updateCloudSuite{})

func (s *updateCloudSuite) SetUpTest(c *gc.C) {
	s.FakeJujuXDGDataHomeSuite.SetUpTest(c)
	s.api = &fakeUpdateCloudAPI{}
	store := jujuclient.NewMemStore()
	store.Controllers["mycontroller"] = jujuclient.ControllerDetails{}
	store.CurrentControllerName = "mycontroller"
	s.store = store
}

func (s *updateCloudSuite) TestBadArgs(c *gc.C) {
	command := cloud.NewUpdateCloudCommandForTest(newFakeCloudMetadataStore(), s.store, nil, "")
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, command, "--controller", "blah")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, command, "--controller", "blah", "-f", "file/path")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, command, "-f", "file/path")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, command, "_a", "file/path")
	c.Assert(err, gc.ErrorMatches, `cloud name "_a" not valid`)

	_, err = cmdtesting.RunCommand(c, command, "boo", "boo")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["boo"\]`)
}

func (s *updateCloudSuite) setupCloudFileScenario(c *gc.C, apiFunc func() (cloud.UpdateCloudAPI, error)) (*cloud.UpdateCloudCommand, string) {
	cloudfile := prepareTestCloudYaml(c, garageMaasYamlFile)
	s.AddCleanup(func(_ *gc.C) {
		defer cloudfile.Close()
		defer os.Remove(cloudfile.Name())
	})
	mockCloud, err := jujucloud.ParseCloudMetadataFile(cloudfile.Name())
	c.Assert(err, jc.ErrorIsNil)
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", cloudfile.Name()).Returns(mockCloud, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("WritePersonalCloudMetadata", mockCloud).Returns(nil)
	command := cloud.NewUpdateCloudCommandForTest(fake, s.store, apiFunc, "")

	return command, cloudfile.Name()
}

func (s *updateCloudSuite) createLocalCacheFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("public-clouds.yaml"), []byte(garageMaasYamlFile), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCloudSuite) TestUpdateLocalCacheFromFile(c *gc.C) {
	command, fileName := s.setupCloudFileScenario(c, func() (cloud.UpdateCloudAPI, error) {
		return nil, errors.New("")
	})
	_, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
}

func (s *updateCloudSuite) TestUpdateFromFileDefaultLocal(c *gc.C) {
	s.store.Controllers = nil
	command, fileName := s.setupCloudFileScenario(c, func() (cloud.UpdateCloudAPI, error) {
		return nil, errors.New("")
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Cloud "garage-maas" updated on this client using provided file.`)
}

func (s *updateCloudSuite) TestUpdateControllerFromFile(c *gc.C) {
	command, fileName := s.setupCloudFileScenario(c, func() (cloud.UpdateCloudAPI, error) {
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Cloud "garage-maas" updated on controller "mycontroller" using provided file.`)
}

func (s *updateCloudSuite) TestUpdateControllerLocalCacheBadFile(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ParseCloudMetadataFile", "somefile.yaml").Returns(map[string]jujucloud.Cloud{}, errors.New("kaboom"))

	addCmd := cloud.NewUpdateCloudCommandForTest(fake, s.store, nil, "")
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "-f", "somefile.yaml", "--controller", "mycontroller")
	c.Check(err, gc.ErrorMatches, "could not read cloud definition from provided file: kaboom")
}

func (s *updateCloudSuite) TestUpdateControllerFromLocalCache(c *gc.C) {
	s.createLocalCacheFile(c)
	command, _ := s.setupCloudFileScenario(c, func() (cloud.UpdateCloudAPI, error) {
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "--controller", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, "Cloud \"garage-maas\" updated on controller \"mycontroller\" using client cloud definition.\n")
}

func (s *updateCloudSuite) TestUpdatePublicCloudDecommissioned(c *gc.C) {
	t := publicCloudsTestData{
		desiredPublishedClouds: sampleUpdateCloudData + `
      anotherregion:
        endpoint: http://anotherregion/1.0
`[1:],
		isClient:      true,
		expectedError: cmd.ErrSilent,
		expectedStderr: `
Fetching latest public cloud list...
ERROR published clouds no longer have a definition for cloud "garage-maas"
`[1:],
	}
	s.assertPublicCloudsChange(c, t)
	s.api.CheckNoCalls(c)
}

func (s *updateCloudSuite) TestUpdatePublicCloudOnClient(c *gc.C) {
	t := publicCloudsTestData{
		desiredPublishedClouds: `
         clouds:
          garage-maas:
             type: maas
             auth-types: [oauth1]
             regions:
              us-east-1:
                endpoint: "https://us-east-1.aws.amazon.com/v1.2/"
              anotherregion:
                endpoint: http://anotherregion/1.0
`[1:],
		expectedStderr: `
Fetching latest public cloud list...
Updated public cloud "garage-maas" on this client, 2 cloud regions added as well as 1 cloud attribute changed:

    added cloud region:
        - garage-maas/anotherregion
        - garage-maas/us-east-1
    changed cloud attribute:
        - garage-maas
`[1:],
		isClient: true,
	}
	s.assertPublicCloudsChange(c, t)
	s.api.CheckNoCalls(c)
}

func (s *updateCloudSuite) TestUpdatePublicCloudOnClientAndController(c *gc.C) {
	t := publicCloudsTestData{
		desiredPublishedClouds: `
         clouds:
          garage-maas:
             type: maas
             auth-types: [oauth1]
             regions:
              us-east-1:
                endpoint: "https://us-east-1.aws.amazon.com/v1.2/"
              anotherregion:
                endpoint: http://anotherregion/1.0
`[1:],
		apiF: func() (cloud.UpdateCloudAPI, error) {
			return s.api, nil
		},
		args: []string{"--controller", "mycontroller"},
		expectedStderr: `
Fetching latest public cloud list...
Updated public cloud "garage-maas" on this client, 2 cloud regions added as well as 1 cloud attribute changed:

    added cloud region:
        - garage-maas/anotherregion
        - garage-maas/us-east-1
    changed cloud attribute:
        - garage-maas
Cloud "garage-maas" updated on controller "mycontroller" using client cloud definition.
`[1:],
		expectedControllerName: "mycontroller",
		isClient:               true,
	}
	s.assertPublicCloudsChange(c, t)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Regions: []jujucloud.Region{
			{Name: "us-east-1", Endpoint: "https://us-east-1.aws.amazon.com/v1.2/"},
			{Name: "anotherregion", Endpoint: "http://anotherregion/1.0"},
		}})
}

type publicCloudsTestData struct {
	desiredPublishedClouds string
	apiF                   func() (cloud.UpdateCloudAPI, error)
	expectedCloud          jujucloud.Cloud
	expectedControllerName string
	expectedError          error
	expectedStderr         string
	isClient               bool
	args                   []string
}

func (s *updateCloudSuite) assertPublicCloudsChange(c *gc.C, t publicCloudsTestData) {
	s.createLocalCacheFile(c)
	ts := setupTestServer(c, t.desiredPublishedClouds)
	defer ts.Close()

	args := []string{"garage-maas", "--client"}
	command := cloud.NewUpdateCloudCommandForTest(nil, s.store, t.apiF, ts.URL)
	ctx, err := cmdtesting.RunCommand(c, command, append(args, t.args...)...)
	if t.expectedError == nil {
		c.Assert(err, jc.ErrorIsNil)
	} else {
		c.Assert(err, gc.Equals, t.expectedError)
	}
	c.Assert(command.ControllerName, gc.Equals, t.expectedControllerName)
	c.Assert(command.Client, gc.Equals, t.isClient)
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, t.expectedStderr)
}

type fakeUpdateCloudAPI struct {
	jujutesting.Stub
}

func (api *fakeUpdateCloudAPI) Close() error {
	api.AddCall("Close", nil)
	return nil
}

func (api *fakeUpdateCloudAPI) UpdateCloud(cloud jujucloud.Cloud) error {
	api.AddCall("UpdateCloud", cloud)
	return api.NextErr()
}
