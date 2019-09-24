// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"os"
	"strings"

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
	cmd := cloud.NewUpdateCloudCommandForTest(newFakeCloudMetadataStore(), s.store, nil)
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, cmd, "--controller", "blah")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, cmd, "--controller", "blah", "-f", "file/path")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, cmd, "-f", "file/path")
	c.Assert(err, gc.ErrorMatches, "cloud name required")

	_, err = cmdtesting.RunCommand(c, cmd, "_a", "file/path")
	c.Assert(err, gc.ErrorMatches, `cloud name "_a" not valid`)

	_, err = cmdtesting.RunCommand(c, cmd, "boo", "boo")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["boo"\]`)
}

func (s *updateCloudSuite) setupCloudFileScenario(c *gc.C, apiFunc func(controllerName string) (cloud.UpdateCloudAPI, error)) (*cloud.UpdateCloudCommand, string) {
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
	cmd := cloud.NewUpdateCloudCommandForTest(fake, s.store, apiFunc)

	return cmd, cloudfile.Name()
}

func (s *updateCloudSuite) createLocalCacheFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("public-clouds.yaml"), []byte(garageMaasYamlFile), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCloudSuite) TestUpdateLocalCacheFromFile(c *gc.C) {
	cmd, fileName := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudAPI, error) {
		return nil, errors.New("")
	})
	_, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "-f", fileName, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
}

func (s *updateCloudSuite) TestUpdateFromFileDefaultLocal(c *gc.C) {
	s.store.Controllers = nil
	cmd, fileName := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudAPI, error) {
		return nil, errors.New("")
	})
	ctx, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "-f", fileName)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `There are no controllers running.Updating cloud on this client so you can use it to bootstrap a controller.*`)
}

func (s *updateCloudSuite) TestUpdateLocalCacheBadFile(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	badFileErr := errors.New("")
	fake.Call("ParseCloudMetadataFile", "somefile.yaml").Returns(map[string]jujucloud.Cloud{}, badFileErr)

	addCmd := cloud.NewUpdateCloudCommandForTest(fake, s.store, nil)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "-f", "somefile.yaml")
	c.Check(errors.Cause(err), gc.Equals, badFileErr)
}

func (s *updateCloudSuite) TestUpdateControllerFromFile(c *gc.C) {
	var controllerNameCalled string
	cmd, fileName := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudAPI, error) {
		controllerNameCalled = controllerName
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "-f", fileName)
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(controllerNameCalled, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Cloud "garage-maas" updated on controller "mycontroller".`)
}

func (s *updateCloudSuite) TestUpdateControllerLocalCacheBadFile(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	badFileErr := errors.New("")
	fake.Call("ParseCloudMetadataFile", "somefile.yaml").Returns(map[string]jujucloud.Cloud{}, badFileErr)

	addCmd := cloud.NewUpdateCloudCommandForTest(fake, s.store, nil)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "-f", "somefile.yaml", "--controller", "mycontroller")
	c.Check(errors.Cause(err), gc.Equals, badFileErr)
}

func (s *updateCloudSuite) TestUpdateControllerFromLocalCache(c *gc.C) {
	s.createLocalCacheFile(c)
	var controllerNameCalled string
	cmd, _ := s.setupCloudFileScenario(c, func(controllerName string) (cloud.UpdateCloudAPI, error) {
		controllerNameCalled = controllerName
		return s.api, nil
	})
	_, err := cmdtesting.RunCommand(c, cmd, "garage-maas", "--controller", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(controllerNameCalled, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:        "garage-maas",
		Type:        "maas",
		Description: "Metal As A Service",
		AuthTypes:   jujucloud.AuthTypes{"oauth1"},
		Endpoint:    "http://garagemaas",
	})
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
