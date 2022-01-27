// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/cmd/v3/cmdtesting"
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
	command := cloud.NewUpdateCloudCommandForTest(newFakeCloudMetadataStore(), s.store, nil)
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

func (s *updateCloudSuite) setupCloudFileScenario(c *gc.C, yamlFile string, apiFunc func() (cloud.UpdateCloudAPI, error)) (*cloud.UpdateCloudCommand, string) {
	clouds, err := jujucloud.ParseCloudMetadata([]byte(yamlFile))
	c.Assert(err, jc.ErrorIsNil)

	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "mycloud.yaml").Returns(yamlFile, nil)
	fake.Call("PublicCloudMetadata", []string(nil)).Returns(map[string]jujucloud.Cloud{}, false, nil)
	fake.Call("PersonalCloudMetadata").Returns(map[string]jujucloud.Cloud{}, nil)
	fake.Call("WritePersonalCloudMetadata", clouds).Returns(nil)
	command := cloud.NewUpdateCloudCommandForTest(fake, s.store, apiFunc)

	return command, "mycloud.yaml"
}

func (s *updateCloudSuite) createLocalCacheFile(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("public-clouds.yaml"), []byte(garageMaasYamlFile), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *updateCloudSuite) TestUpdateLocalCacheFromFile(c *gc.C) {
	command, fileName := s.setupCloudFileScenario(c, garageMaasYamlFile, func() (cloud.UpdateCloudAPI, error) {
		return nil, errors.New("")
	})
	_, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "--client")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.api.Calls(), gc.HasLen, 0)
}

func (s *updateCloudSuite) TestUpdateFromFileDefaultLocal(c *gc.C) {
	s.store.Controllers = nil
	command, fileName := s.setupCloudFileScenario(c, garageMaasYamlFile, func() (cloud.UpdateCloudAPI, error) {
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
	command, fileName := s.setupCloudFileScenario(c, garageMaasYamlFile, func() (cloud.UpdateCloudAPI, error) {
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:          "garage-maas",
		Type:          "maas",
		Description:   "Metal As A Service",
		AuthTypes:     jujucloud.AuthTypes{"oauth1"},
		Endpoint:      "http://garagemaas",
		SkipTLSVerify: true,
	})
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Cloud "garage-maas" updated on controller "mycontroller" using provided file.`)
}

func (s *updateCloudSuite) TestUpdateControllerFromFileWithoutCloudsKeyword(c *gc.C) {
	command, fileName := s.setupCloudFileScenario(c, garageMaasYamlFileListCloudOutput, func() (cloud.UpdateCloudAPI, error) {
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "-f", fileName, "-c", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:          "garage-maas",
		Type:          "maas",
		Description:   "Metal As A Service",
		AuthTypes:     jujucloud.AuthTypes{"oauth1"},
		Endpoint:      "http://garagemaas",
		SkipTLSVerify: true,
	})
	out := cmdtesting.Stderr(ctx)
	out = strings.Replace(out, "\n", "", -1)
	c.Assert(out, gc.Matches, `Cloud "garage-maas" updated on controller "mycontroller" using provided file.`)
}

func (s *updateCloudSuite) TestUpdateControllerLocalCacheBadFile(c *gc.C) {
	fake := newFakeCloudMetadataStore()
	fake.Call("ReadCloudData", "somefile.yaml").Returns(nil, errors.New("kaboom"))

	addCmd := cloud.NewUpdateCloudCommandForTest(fake, s.store, nil)
	_, err := cmdtesting.RunCommand(c, addCmd, "cloud", "-f", "somefile.yaml", "--controller", "mycontroller")
	c.Check(err, gc.ErrorMatches, "could not read cloud definition from provided file: kaboom")
}

func (s *updateCloudSuite) TestUpdateControllerFromLocalCache(c *gc.C) {
	s.createLocalCacheFile(c)
	command, _ := s.setupCloudFileScenario(c, garageMaasYamlFile, func() (cloud.UpdateCloudAPI, error) {
		return s.api, nil
	})
	ctx, err := cmdtesting.RunCommand(c, command, "garage-maas", "--controller", "mycontroller")
	c.Assert(err, jc.ErrorIsNil)
	s.api.CheckCallNames(c, "UpdateCloud", "Close")
	c.Assert(command.ControllerName, gc.Equals, "mycontroller")
	s.api.CheckCall(c, 0, "UpdateCloud", jujucloud.Cloud{
		Name:          "garage-maas",
		Type:          "maas",
		Description:   "Metal As A Service",
		AuthTypes:     jujucloud.AuthTypes{"oauth1"},
		Endpoint:      "http://garagemaas",
		SkipTLSVerify: true,
	})
	c.Assert(cmdtesting.Stdout(ctx), gc.Equals, "")
	c.Assert(cmdtesting.Stderr(ctx), gc.Equals, `
To ensure this client's copy or any controller copies of public cloud information is up to date with the latest region information, use `[1:]+"`juju update-public-clouds`"+`.
Cloud "garage-maas" updated on controller "mycontroller" using client cloud definition.
`)
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
