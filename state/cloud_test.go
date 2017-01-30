// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
)

type CloudSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudSuite{})

var lowCloud = cloud.Cloud{
	Name:             "stratus",
	Type:             "low",
	AuthTypes:        cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	Endpoint:         "global-endpoint",
	IdentityEndpoint: "identity-endpoint",
	StorageEndpoint:  "storage-endpoint",
	Regions: []cloud.Region{{
		Name:             "region1",
		Endpoint:         "region1-endpoint",
		IdentityEndpoint: "region1-identity",
		StorageEndpoint:  "region1-storage",
	}, {
		Name:             "region2",
		Endpoint:         "region2-endpoint",
		IdentityEndpoint: "region2-identity",
		StorageEndpoint:  "region2-storage",
	}},
}

func (s *CloudSuite) TestCloudNotFound(c *gc.C) {
	cld, err := s.State.Cloud("unknown")
	c.Assert(err, gc.ErrorMatches, `cloud "unknown" not found`)
	c.Assert(cld, jc.DeepEquals, cloud.Cloud{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudSuite) TestClouds(c *gc.C) {
	dummyCloud, err := s.State.Cloud("dummy")
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AddCloud(lowCloud)
	c.Assert(err, jc.ErrorIsNil)

	clouds, err := s.State.Clouds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(clouds, jc.DeepEquals, map[names.CloudTag]cloud.Cloud{
		names.NewCloudTag("dummy"):   dummyCloud,
		names.NewCloudTag("stratus"): lowCloud,
	})
}

func (s *CloudSuite) TestAddCloud(c *gc.C) {
	err := s.State.AddCloud(lowCloud)
	c.Assert(err, jc.ErrorIsNil)
	cloud, err := s.State.Cloud("stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cloud, jc.DeepEquals, lowCloud)
}

func (s *CloudSuite) TestAddCloudDuplicate(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, gc.ErrorMatches, `cloud "stratus" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *CloudSuite) TestAddCloudNoName(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty Name not valid`)
}

func (s *CloudSuite) TestAddCloudNoType(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name:      "stratus",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty Type not valid`)
}

func (s *CloudSuite) TestAddCloudNoAuthTypes(c *gc.C) {
	err := s.State.AddCloud(cloud.Cloud{
		Name: "stratus",
		Type: "foo",
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)
}
