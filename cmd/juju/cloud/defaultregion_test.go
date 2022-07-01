// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/cmd/v3/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/v2/cloud"
	"github.com/juju/juju/v2/cmd/juju/cloud"
	"github.com/juju/juju/v2/jujuclient"
	"github.com/juju/juju/v2/testing"
)

type defaultRegionSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&defaultRegionSuite{})

func (s *defaultRegionSuite) TestBadArgs(c *gc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, gc.ErrorMatches, `Usage: juju default-region <cloud-name> \[<region>\]`)
	_, err = cmdtesting.RunCommand(c, command, "cloud", "region", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultRegionSuite) TestBadRegion(c *gc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `region "foo" not found .*`)
}

func (s *defaultRegionSuite) TestBadCloudName(c *gc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command, "somecloud", "us-west-1")
	c.Assert(err, gc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultRegionSuite) assertSetDefaultRegion(c *gc.C, cmd cmd.Command, store *jujuclient.MemStore, cloud, errStr string) {
	s.assertSetCustomDefaultRegion(c, cmd, store, cloud, "us-west-1", errStr)
}

func (s *defaultRegionSuite) assertSetCustomDefaultRegion(c *gc.C, cmd cmd.Command, store *jujuclient.MemStore, cloud, desiredDefault, errStr string) {
	ctx, err := cmdtesting.RunCommand(c, cmd, cloud, desiredDefault)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	if errStr != "" {
		c.Assert(err, gc.ErrorMatches, errStr)
		c.Assert(output, gc.Equals, "")
		c.Assert(store.Credentials[cloud].DefaultRegion, gc.Equals, "")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default region in %s set to %q.`, cloud, desiredDefault))
	c.Assert(store.Credentials[cloud].DefaultRegion, gc.Equals, desiredDefault)
}

func (s *defaultRegionSuite) TestSetDefaultRegion(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		}}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, command, store, "aws", "")
}

func (s *defaultRegionSuite) TestSetDefaultRegionBuiltIn(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["localhost"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		}}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	// Cloud 'localhost' is of type lxd.
	s.assertSetCustomDefaultRegion(c, command, store, "localhost", "localhost", "")
}

func (s *defaultRegionSuite) TestOverwriteDefaultRegion(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		},
		DefaultRegion: "us-east-1"}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, command, store, "aws", "")
}

func (s *defaultRegionSuite) TestCaseInsensitiveRegionSpecification(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		},
		DefaultRegion: "us-east-1"}

	command := cloud.NewSetDefaultRegionCommandForTest(store)
	_, err := cmdtesting.RunCommand(c, command, "aws", "us-WEST-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Credentials["aws"].DefaultRegion, gc.Equals, "us-west-1")
}

func (s *defaultRegionSuite) TestReadDefaultRegion(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultRegion: "us-east-1",
	}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, command, cloudName)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default region for cloud %q is "us-east-1" on this client.`, cloudName))
}

func (s *defaultRegionSuite) TestReadDefaultRegionNoneSet(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, command, cloudName)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default region for cloud %q is not set on this client.`, cloudName))
}

func (s *defaultRegionSuite) TestResetDefaultRegion(c *gc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{
		DefaultRegion: "us-east-1",
	}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, command, cloudName, "--reset")
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, gc.Equals, fmt.Sprintf(`Default region for cloud %q is no longer set on this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, gc.Equals, "")
}
