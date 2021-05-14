// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
)

func (s *cloudSuite) TestWhitelistString(c *gc.C) {
	c.Assert((&cloud.WhiteList{}).String(), gc.Equals, "empty whitelist")
	c.Assert(cloud.CurrentWhiteList().String(), gc.Equals, `
 - controller cloud type "kubernetes" supports [lxd maas openstack]
 - controller cloud type "lxd" supports [lxd maas openstack]
 - controller cloud type "maas" supports [maas openstack]
 - controller cloud type "openstack" supports [openstack]`[1:])
}

func (s *cloudSuite) TestCheckWhitelistSuccess(c *gc.C) {
	c.Assert(cloud.CurrentWhiteList().Check("maas", "maas"), jc.ErrorIsNil)
}

func (s *cloudSuite) TestCheckWhitelistFail(c *gc.C) {
	c.Assert(cloud.CurrentWhiteList().Check("ec2", "maas"), gc.ErrorMatches, `
controller cloud type "ec2" is not whitelisted, current whitelist: 
 - controller cloud type "kubernetes" supports \[lxd maas openstack\]
 - controller cloud type "lxd" supports \[lxd maas openstack\]
 - controller cloud type "maas" supports \[maas openstack\]
 - controller cloud type "openstack" supports \[openstack\]`[1:])

	c.Assert(cloud.CurrentWhiteList().Check("openstack", "ec2"), gc.ErrorMatches,
		`cloud type "ec2" is not whitelisted for controller cloud type "openstack", current whitelist: \[openstack\]`)
}
