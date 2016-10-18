// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujucloud "github.com/juju/juju/cloud"
	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
)

type removeSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&removeSuite{})

func (s *removeSuite) TestRemoveBadArgs(c *gc.C) {
	cmd := cloud.NewRemoveCloudCommand()
	_, err := testing.RunCommand(c, cmd)
	c.Assert(err, gc.ErrorMatches, "Usage: juju remove-cloud <cloud name>")
	_, err = testing.RunCommand(c, cmd, "cloud", "extra")
	c.Assert(err, gc.ErrorMatches, `unrecognized args: \["extra"\]`)
}

func (s *removeSuite) TestRemoveNotFound(c *gc.C) {
	cmd := cloud.NewRemoveCloudCommand()
	ctx, err := testing.RunCommand(c, cmd, "fnord")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "No personal cloud called \"fnord\" exists\n")
}

func (s *removeSuite) createTestCloudData(c *gc.C) {
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("public-clouds.yaml"), []byte(`
clouds:
  prodstack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
`[1:]), 0600)
	c.Assert(err, jc.ErrorIsNil)

	err = ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(`
clouds:
  homestack:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack
  homestack2:
    type: openstack
    auth-types: [userpass, access-key]
    endpoint: http://homestack2
`[1:]), 0600)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *removeSuite) TestRemoveCloud(c *gc.C) {
	s.createTestCloudData(c)
	assertPersonalClouds(c, "homestack", "homestack2")
	ctx, err := testing.RunCommand(c, cloud.NewRemoveCloudCommand(), "homestack")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "Removed details of personal cloud \"homestack\"\n")
	assertPersonalClouds(c, "homestack2")
}

func (s *removeSuite) TestCannotRemovePublicCloud(c *gc.C) {
	s.createTestCloudData(c)
	ctx, err := testing.RunCommand(c, cloud.NewRemoveCloudCommand(), "prodstack")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(testing.Stderr(ctx), gc.Equals, "No personal cloud called \"prodstack\" exists\n")
}

func assertPersonalClouds(c *gc.C, names ...string) {
	personalClouds, err := jujucloud.PersonalCloudMetadata()
	c.Assert(err, jc.ErrorIsNil)
	actual := make([]string, 0, len(personalClouds))
	for name := range personalClouds {
		actual = append(actual, name)
	}
	c.Assert(actual, jc.SameContents, names)
}
