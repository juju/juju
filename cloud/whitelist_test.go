// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud_test

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
)

func (s *cloudSuite) TestWhitelistString(c *tc.C) {
	c.Assert((&cloud.WhiteList{}).String(), tc.Equals, "empty whitelist")
	c.Assert(cloud.CurrentWhiteList().String(), tc.Equals, `
 - controller cloud type "kubernetes" supports [lxd maas openstack]
 - controller cloud type "lxd" supports [lxd maas openstack]
 - controller cloud type "maas" supports [maas openstack]
 - controller cloud type "openstack" supports [openstack]`[1:])
}

func (s *cloudSuite) TestCheckWhitelistSuccess(c *tc.C) {
	c.Assert(cloud.CurrentWhiteList().Check("maas", "maas"), jc.ErrorIsNil)
}

func (s *cloudSuite) TestCheckWhitelistFail(c *tc.C) {
	c.Assert(cloud.CurrentWhiteList().Check("ec2", "maas"), tc.ErrorMatches, `
controller cloud type "ec2" is not whitelisted, current whitelist: 
 - controller cloud type "kubernetes" supports \[lxd maas openstack\]
 - controller cloud type "lxd" supports \[lxd maas openstack\]
 - controller cloud type "maas" supports \[maas openstack\]
 - controller cloud type "openstack" supports \[openstack\]`[1:])

	c.Assert(cloud.CurrentWhiteList().Check("openstack", "ec2"), tc.ErrorMatches,
		`cloud type "ec2" is not whitelisted for controller cloud type "openstack", current whitelist: \[openstack\]`)
}
