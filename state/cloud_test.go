// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
)

type CloudSuite struct {
	ConnSuite
}

var _ = gc.Suite(&CloudSuite{})

func (s *CloudSuite) TestCloudNotFound(c *gc.C) {
	cld, err := s.State.Cloud("unknown")
	c.Assert(err, gc.ErrorMatches, `cloud "unknown" not found`)
	c.Assert(cld, jc.DeepEquals, cloud.Cloud{})
	c.Assert(err, jc.Satisfies, errors.IsNotFound)
}

func (s *CloudSuite) TestAddCloud(c *gc.C) {
	cld := cloud.Cloud{
		Type:            "low",
		AuthTypes:       cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
		Endpoint:        "global-endpoint",
		StorageEndpoint: "global-storage",
		Regions: []cloud.Region{{
			Name:            "region1",
			Endpoint:        "region1-endpoint",
			StorageEndpoint: "region1-storage",
		}, {
			Name:            "region2",
			Endpoint:        "region2-endpoint",
			StorageEndpoint: "region2-storage",
		}},
	}
	err := s.State.AddCloud("stratus", cld)
	c.Assert(err, jc.ErrorIsNil)
	cld1, err := s.State.Cloud("stratus")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(cld1, jc.DeepEquals, cld)
}

func (s *CloudSuite) TestAddCloudDuplicate(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, jc.ErrorIsNil)
	err = s.State.AddCloud("stratus", cloud.Cloud{
		Type:      "low",
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, gc.ErrorMatches, `cloud "stratus" already exists`)
	c.Assert(err, jc.Satisfies, errors.IsAlreadyExists)
}

func (s *CloudSuite) TestAddCloudNoType(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		AuthTypes: cloud.AuthTypes{cloud.AccessKeyAuthType, cloud.UserPassAuthType},
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty Type not valid`)
}

func (s *CloudSuite) TestAddCloudNoAuthTypes(c *gc.C) {
	err := s.State.AddCloud("stratus", cloud.Cloud{
		Type: "foo",
	})
	c.Assert(err, gc.ErrorMatches, `invalid cloud: empty auth-types not valid`)
}
