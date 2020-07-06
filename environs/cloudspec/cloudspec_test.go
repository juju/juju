// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
package cloudspec_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	environscloudspec "github.com/juju/juju/environs/cloudspec"
)

type cloudSpecSuite struct {
}

var _ = gc.Suite(&cloudSpecSuite{})

func (s *cloudSpecSuite) TestNewRegionSpec(c *gc.C) {
	tests := []struct {
		description, cloud, region, errMatch string
		nilErr                               bool
		want                                 *environscloudspec.CloudRegionSpec
	}{
		{
			description: "test empty cloud",
			cloud:       "",
			region:      "aregion",
			errMatch:    "cloud is required to be non empty",
			want:        nil,
		}, {
			description: "test empty region",
			cloud:       "acloud",
			region:      "",
			nilErr:      true,
			want:        &environscloudspec.CloudRegionSpec{Cloud: "acloud"},
		}, {
			description: "test valid",
			cloud:       "acloud",
			region:      "aregion",
			nilErr:      true,
			want:        &environscloudspec.CloudRegionSpec{Cloud: "acloud", Region: "aregion"},
		},
	}
	for i, test := range tests {
		c.Logf("Test %d: %s", i, test.description)
		rspec, err := environscloudspec.NewCloudRegionSpec(test.cloud, test.region)
		if !test.nilErr {
			c.Check(err, gc.ErrorMatches, test.errMatch)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
		c.Check(rspec, jc.DeepEquals, test.want)
	}
}
