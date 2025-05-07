// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"sort"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
}

var _ = tc.Suite(&cloudSuite{})

func (*cloudSuite) TestFinalizeCloudSetAuthTypes(c *tc.C) {
	environCloud := environProviderCloud{}
	r, err := environCloud.FinalizeCloud(nil, cloud.Cloud{})
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(r.AuthTypes)
	c.Assert(r.AuthTypes, jc.DeepEquals, cloud.AuthTypes{"instance-role"})
}

func (*cloudSuite) TestFinalizeCloudSetAuthTypesAddition(c *tc.C) {
	environCloud := environProviderCloud{}
	r, err := environCloud.FinalizeCloud(nil, cloud.Cloud{AuthTypes: cloud.AuthTypes{"test"}})
	c.Assert(err, jc.ErrorIsNil)
	sort.Sort(r.AuthTypes)
	c.Assert(r.AuthTypes, jc.DeepEquals, cloud.AuthTypes{"instance-role", "test"})
}
