// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/internal/cmd"
	"github.com/juju/juju/internal/cmd/cmdtesting"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/jujuclient"
)

type defaultRegionSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = tc.Suite(&defaultRegionSuite{})

func (s *defaultRegionSuite) TestBadArgs(c *tc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command)
	c.Assert(err, tc.ErrorMatches, `Usage: juju default-region <cloud-name> \[<region>\]`)
	_, err = cmdtesting.RunCommand(c, command, "cloud", "region", "extra")
	c.Assert(err, tc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultRegionSuite) TestBadRegion(c *tc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command, "aws", "foo")
	c.Assert(err, tc.ErrorMatches, `region "foo" not found .*`)
}

func (s *defaultRegionSuite) TestBadCloudName(c *tc.C) {
	command := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, command, "somecloud", "us-west-1")
	c.Assert(err, tc.ErrorMatches, `cloud somecloud not valid`)
}

func (s *defaultRegionSuite) assertSetDefaultRegion(c *tc.C, cmd cmd.Command, store *jujuclient.MemStore, cloud, errStr string) {
	s.assertSetCustomDefaultRegion(c, cmd, store, cloud, "us-west-1", errStr)
}

func (s *defaultRegionSuite) assertSetCustomDefaultRegion(c *tc.C, cmd cmd.Command, store *jujuclient.MemStore, cloud, desiredDefault, errStr string) {
	ctx, err := cmdtesting.RunCommand(c, cmd, cloud, desiredDefault)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	if errStr != "" {
		c.Assert(err, tc.ErrorMatches, errStr)
		c.Assert(output, tc.Equals, "")
		c.Assert(store.Credentials[cloud].DefaultRegion, tc.Equals, "")
		return
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default region in %s set to %q.`, cloud, desiredDefault))
	c.Assert(store.Credentials[cloud].DefaultRegion, tc.Equals, desiredDefault)
}

func (s *defaultRegionSuite) TestSetDefaultRegion(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		}}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, command, store, "aws", "")
}

func (s *defaultRegionSuite) TestSetDefaultRegionBuiltIn(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["localhost"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		}}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	// Cloud 'localhost' is of type lxd.
	s.assertSetCustomDefaultRegion(c, command, store, "localhost", "localhost", "")
}

func (s *defaultRegionSuite) TestOverwriteDefaultRegion(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		},
		DefaultRegion: "us-east-1"}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, command, store, "aws", "")
}

func (s *defaultRegionSuite) TestCaseInsensitiveRegionSpecification(c *tc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": {},
		},
		DefaultRegion: "us-east-1"}

	command := cloud.NewSetDefaultRegionCommandForTest(store)
	_, err := cmdtesting.RunCommand(c, command, "aws", "us-WEST-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Credentials["aws"].DefaultRegion, tc.Equals, "us-west-1")
}

func (s *defaultRegionSuite) TestReadDefaultRegion(c *tc.C) {
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
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default region for cloud %q is "us-east-1" on this client.`, cloudName))
}

func (s *defaultRegionSuite) TestReadDefaultRegionNoneSet(c *tc.C) {
	cloudName := "aws"
	store := jujuclient.NewMemStore()
	store.Credentials[cloudName] = jujucloud.CloudCredential{}
	command := cloud.NewSetDefaultRegionCommandForTest(store)
	ctx, err := cmdtesting.RunCommand(c, command, cloudName)
	c.Assert(err, jc.ErrorIsNil)
	output := cmdtesting.Stderr(ctx)
	output = strings.Replace(output, "\n", "", -1)
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default region for cloud %q is not set on this client.`, cloudName))
}

func (s *defaultRegionSuite) TestResetDefaultRegion(c *tc.C) {
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
	c.Assert(output, tc.Equals, fmt.Sprintf(`Default region for cloud %q is no longer set on this client.`, cloudName))
	c.Assert(store.Credentials[cloudName].DefaultCredential, tc.Equals, "")
}
