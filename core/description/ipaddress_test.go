// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type IPAddressSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&IPAddressSerializationSuite{})

func (s *IPAddressSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "addresss"
	s.sliceName = "addresss"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importIPAddresss(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["addresss"] = []interface{}{}
	}
}

func (s *IPAddressSerializationSuite) TestNewIPAddress(c *gc.C) {
	args := IPAddressArgs{
		CIDR:              "10.0.0.0/24",
		ProviderId:        "magic",
		VLANTag:           64,
		SpaceName:         "foo",
		AvailabilityZone:  "bar",
		AllocatableIPHigh: "10.0.0.255",
		AllocatableIPLow:  "10.0.0.0",
	}
	address := newIPAddress(args)
	c.Assert(address.CIDR(), gc.Equals, args.CIDR)
	c.Assert(address.ProviderId(), gc.Equals, args.ProviderId)
	c.Assert(address.VLANTag(), gc.Equals, args.VLANTag)
	c.Assert(address.SpaceName(), gc.Equals, args.SpaceName)
	c.Assert(address.AvailabilityZone(), gc.Equals, args.AvailabilityZone)
	c.Assert(address.AllocatableIPHigh(), gc.Equals, args.AllocatableIPHigh)
	c.Assert(address.AllocatableIPLow(), gc.Equals, args.AllocatableIPLow)
}

func (s *IPAddressSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := addresss{
		Version: 1,
		IPAddresss_: []*address{
			newIPAddress(IPAddressArgs{
				CIDR:              "10.0.0.0/24",
				ProviderId:        "magic",
				VLANTag:           64,
				SpaceName:         "foo",
				AvailabilityZone:  "bar",
				AllocatableIPHigh: "10.0.0.255",
				AllocatableIPLow:  "10.0.0.0",
			}),
			newIPAddress(IPAddressArgs{CIDR: "10.0.1.0/24"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	addresss, err := importIPAddresss(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addresss, jc.DeepEquals, initial.IPAddresss_)
}
