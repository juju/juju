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
	s.importName = "ip-addresses"
	s.sliceName = "ip-addresses"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importIPAddresses(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["ip-addresses"] = []interface{}{}
	}
}

func (s *IPAddressSerializationSuite) TestNewIPAddress(c *gc.C) {
	args := IPAddressArgs{
		SubnetCIDR:       "10.0.0.0/24",
		ProviderID:       "magic",
		DeviceName:       "foo",
		MachineID:        "bar",
		ConfigMethod:     "static",
		Value:            "10.0.0.4",
		DNSServers:       []string{"10.1.0.1", "10.2.0.1"},
		DNSSearchDomains: []string{"bam", "mam"},
		GatewayAddress:   "10.0.0.1",
	}
	address := newIPAddress(args)
	c.Assert(address.SubnetCIDR(), gc.Equals, args.SubnetCIDR)
	c.Assert(address.ProviderID(), gc.Equals, args.ProviderID)
	c.Assert(address.DeviceName(), gc.Equals, args.DeviceName)
	c.Assert(address.MachineID(), gc.Equals, args.MachineID)
	c.Assert(address.ConfigMethod(), gc.Equals, args.ConfigMethod)
	c.Assert(address.Value(), gc.Equals, args.Value)
	c.Assert(address.DNSServers(), jc.DeepEquals, args.DNSServers)
	c.Assert(address.DNSSearchDomains(), jc.DeepEquals, args.DNSSearchDomains)
	c.Assert(address.GatewayAddress(), gc.Equals, args.GatewayAddress)
}

func (s *IPAddressSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := ipaddresses{
		Version: 1,
		IPAddresses_: []*ipaddress{
			newIPAddress(IPAddressArgs{
				SubnetCIDR:       "10.0.0.0/24",
				ProviderID:       "magic",
				DeviceName:       "foo",
				MachineID:        "bar",
				ConfigMethod:     "static",
				Value:            "10.0.0.4",
				DNSServers:       []string{"10.1.0.1", "10.2.0.1"},
				DNSSearchDomains: []string{"bam", "mam"},
				GatewayAddress:   "10.0.0.1",
			}),
			newIPAddress(IPAddressArgs{Value: "10.0.0.5"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	addresses, err := importIPAddresses(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(addresses, jc.DeepEquals, initial.IPAddresses_)
}
