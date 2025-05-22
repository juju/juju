// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2

import (
	"sort"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
)

type cloudSuite struct {
}

func TestCloudSuite(t *testing.T) {
	tc.Run(t, &cloudSuite{})
}

func (*cloudSuite) TestFinalizeCloudSetAuthTypes(c *tc.C) {
	environCloud := environProviderCloud{}
	r, err := environCloud.FinalizeCloud(nil, cloud.Cloud{})
	c.Assert(err, tc.ErrorIsNil)
	sort.Sort(r.AuthTypes)
	c.Assert(r.AuthTypes, tc.DeepEquals, cloud.AuthTypes{"instance-role"})
}

func (*cloudSuite) TestFinalizeCloudSetAuthTypesAddition(c *tc.C) {
	environCloud := environProviderCloud{}
	r, err := environCloud.FinalizeCloud(nil, cloud.Cloud{AuthTypes: cloud.AuthTypes{"test"}})
	c.Assert(err, tc.ErrorIsNil)
	sort.Sort(r.AuthTypes)
	c.Assert(r.AuthTypes, tc.DeepEquals, cloud.AuthTypes{"instance-role", "test"})
}
