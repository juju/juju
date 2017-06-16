// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"io/ioutil"
	"strings"

	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/cloud"
	"github.com/juju/juju/juju/osenv"
	_ "github.com/juju/juju/provider/all"
	"github.com/juju/juju/testing"
)

type showSuite struct {
	testing.FakeJujuXDGDataHomeSuite
}

var _ = gc.Suite(&showSuite{})

func (s *showSuite) TestShowBadArgs(c *gc.C) {
	_, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand())
	c.Assert(err, gc.ErrorMatches, "no cloud specified")
}

func (s *showSuite) TestShow(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "aws-china")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
defined: public
type: ec2
description: Amazon China
auth-types: [access-key]
regions:
  cn-north-1:
    endpoint: https://ec2.cn-north-1.amazonaws.com.cn
`[1:])
}

func (s *showSuite) TestShowWithConfig(c *gc.C) {
	data := `
clouds:
  homestack:
    type: openstack
    description: Openstack Cloud
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
    config:
      bootstrap-timeout: 1800
      use-default-secgroup: true
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)

	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
defined: local
type: openstack
description: Openstack Cloud
auth-types: [userpass, access-key]
endpoint: http://homestack
regions:
  london:
    endpoint: http://london/1.0
config:
  bootstrap-timeout: 1800
  use-default-secgroup: true
`[1:])
}

var openstackProviderConfig = `
The available config options specific to openstack clouds are:
external-network:
  type: string
  description: The network label or UUID to create floating IP addresses on when multiple
    external networks exist.
network:
  type: string
  description: The network label or UUID to bring machines up on when multiple networks
    exist.
use-default-secgroup:
  type: bool
  description: Whether new machine instances should have the "default" Openstack security
    group assigned in addition to juju defined security groups.
use-floating-ip:
  type: bool
  description: Whether a floating IP address is required to give the nodes a public
    IP address. Some installations assign public IP addresses by default without requiring
    a floating IP address.
`

func (s *showSuite) TestShowWithRegionConfigAndFlags(c *gc.C) {
	data := `
clouds:
  homestack:
    type: openstack
    description: Openstack Cloud
    auth-types: [userpass, access-key]
    endpoint: http://homestack
    regions:
      london:
        endpoint: http://london/1.0
    config:
      bootstrap-retry-delay: 1500
      network: nameme
    region-config:
      london:
        bootstrap-timeout: 1800
        use-floating-ip: true
`[1:]
	err := ioutil.WriteFile(osenv.JujuXDGDataHomePath("clouds.yaml"), []byte(data), 0600)

	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "homestack", "--include-config")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, strings.Join([]string{`defined: local
type: openstack
description: Openstack Cloud
auth-types: [userpass, access-key]
endpoint: http://homestack
regions:
  london:
    endpoint: http://london/1.0
config:
  bootstrap-retry-delay: 1500
  network: nameme
region-config:
  london:
    bootstrap-timeout: 1800
    use-floating-ip: true
`, openstackProviderConfig}, ""))
}

func (s *showSuite) TestShowWithRegionConfigAndFlagNoExtraOut(c *gc.C) {
	ctx, err := cmdtesting.RunCommand(c, cloud.NewShowCloudCommand(), "joyent", "--include-config")
	c.Assert(err, jc.ErrorIsNil)
	out := cmdtesting.Stdout(ctx)
	c.Assert(out, gc.Equals, `
defined: public
type: joyent
description: Joyent Cloud
auth-types: [userpass]
regions:
  eu-ams-1:
    endpoint: https://eu-ams-1.api.joyentcloud.com
  us-sw-1:
    endpoint: https://us-sw-1.api.joyentcloud.com
  us-east-1:
    endpoint: https://us-east-1.api.joyentcloud.com
  us-east-2:
    endpoint: https://us-east-2.api.joyentcloud.com
  us-east-3:
    endpoint: https://us-east-3.api.joyentcloud.com
  us-west-1:
    endpoint: https://us-west-1.api.joyentcloud.com
`[1:])
}
