// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type SubnetSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&SubnetSerializationSuite{})

func (s *SubnetSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "subnets"
	s.sliceName = "subnets"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importSubnets(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["subnets"] = []interface{}{}
	}
}

func (s *SubnetSerializationSuite) TestNewSubnet(c *gc.C) {
	args := SubnetArgs{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "magic",
		VLANTag:           64,
		SpaceName:         "foo",
		AvailabilityZone:  "bar",
		AllocatableIPHigh: "10.0.0.255",
		AllocatableIPLow:  "10.0.0.0",
	}
	subnet := newSubnet(args)
	c.Assert(subnet.CIDR(), gc.Equals, args.CIDR)
	c.Assert(subnet.ProviderId(), gc.Equals, args.ProviderId)
	c.Assert(subnet.VLANTag(), gc.Equals, args.VLANTag)
	c.Assert(subnet.SpaceName(), gc.Equals, args.SpaceName)
	c.Assert(subnet.AvailabilityZone(), gc.Equals, args.AvailabilityZone)
	c.Assert(subnet.AllocatableIPHigh(), gc.Equals, args.AllocatableIPHigh)
	c.Assert(subnet.AllocatableIPLow(), gc.Equals, args.AllocatableIPLow)
}

func (s *SubnetSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := subnets{
		Version: 1,
		Subnets_: []*subnet{
			newSubnet(SubnetArgs{
				CIDR:              "10.0.0.0/24",
				ProviderId:        "magic",
				VLANTag:           64,
				SpaceName:         "foo",
				AvailabilityZone:  "bar",
				AllocatableIPHigh: "10.0.0.255",
				AllocatableIPLow:  "10.0.0.0",
			}),
			newSubnet(SubnetArgs{CIDR: "10.0.1.0/24"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	subnets, err := importSubnets(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(subnets, jc.DeepEquals, initial.Subnets_)
}
