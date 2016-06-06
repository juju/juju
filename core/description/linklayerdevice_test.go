// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type LinkLayerDeviceSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&LinkLayerDeviceSerializationSuite{})

func (s *LinkLayerDeviceSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "linklayerdevices"
	s.sliceName = "linklayerdevices"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importLinkLayerDevices(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["linklayerdevices"] = []interface{}{}
	}
}

func (s *LinkLayerDeviceSerializationSuite) TestNewLinkLayerDevice(c *gc.C) {
	args := LinkLayerDeviceArgs{
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
	device := newLinkLayerDevice(args)
	c.Assert(device.SubnetCIDR(), gc.Equals, args.SubnetCIDR)
	c.Assert(device.ProviderID(), gc.Equals, args.ProviderID)
	c.Assert(device.DeviceName(), gc.Equals, args.DeviceName)
	c.Assert(device.MachineID(), gc.Equals, args.MachineID)
	c.Assert(device.ConfigMethod(), gc.Equals, args.ConfigMethod)
	c.Assert(device.Value(), gc.Equals, args.Value)
	c.Assert(device.DNSServers(), jc.DeepEquals, args.DNSServers)
	c.Assert(device.DNSSearchDomains(), jc.DeepEquals, args.DNSSearchDomains)
	c.Assert(device.GatewayAddress(), gc.Equals, args.GatewayAddress)
}

func (s *LinkLayerDeviceSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := linklayerdevices{
		Version: 1,
		LinkLayerDevices_: []*linklayerdevice{
			newLinkLayerDevice(LinkLayerDeviceArgs{
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
			newLinkLayerDevice(LinkLayerDeviceArgs{Value: "10.0.0.5"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	devices, err := importLinkLayerDevices(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(devices, jc.DeepEquals, initial.LinkLayerDevices_)
}
