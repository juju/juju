// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"fmt"
	"strings"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/testing"
)

type defaultRegionSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&defaultRegionSuite{})

func (s *defaultRegionSuite) TestBadArgs(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju set-default-region <cloud-name> <region>")
	_, err = cmdtesting.RunCommand(c, cmd, "cloud", "region", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *defaultRegionSuite) TestBadRegion(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "aws", "foo")
	c.Assert(err, gc.ErrorMatches, `region "foo" for cloud aws not valid, valid regions are .*`)
}

func (s *defaultRegionSuite) TestBadCloudName(c *gc.C) {
	cmd := cloud.NewSetDefaultRegionCommand()
	_, err := cmdtesting.RunCommand(c, cmd, "somecloud", "us-west-1")
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
			"one": jujucloud.Credential{},
		}}
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store, "aws", "")
}

func (s *defaultRegionSuite) TestSetDefaultRegionBuiltIn(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["localhost"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": jujucloud.Credential{},
		}}
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	// Cloud 'localhost' is of type lxd.
	s.assertSetCustomDefaultRegion(c, cmd, store, "localhost", "localhost", "")
}

func (s *defaultRegionSuite) TestOverwriteDefaultRegion(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": jujucloud.Credential{},
		},
		DefaultRegion: "us-east-1"}
	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	s.assertSetDefaultRegion(c, cmd, store, "aws", "")
}

func (s *defaultRegionSuite) TestCaseInsensitiveRegionSpecification(c *gc.C) {
	store := jujuclient.NewMemStore()
	store.Credentials["aws"] = jujucloud.CloudCredential{
		AuthCredentials: map[string]jujucloud.Credential{
			"one": jujucloud.Credential{},
		},
		DefaultRegion: "us-east-1"}

	cmd := cloud.NewSetDefaultRegionCommandForTest(store)
	_, err := cmdtesting.RunCommand(c, cmd, "aws", "us-WEST-1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(store.Credentials["aws"].DefaultRegion, gc.Equals, "us-west-1")
}
