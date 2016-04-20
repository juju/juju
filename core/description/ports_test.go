// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type PortRangeCheck struct{}

func (*PortRangeCheck) AssertPortRange(c *gc.C, pr PortRange, args PortRangeArgs) {
	c.Assert(pr.UnitName(), gc.Equals, args.UnitName)
	c.Assert(pr.FromPort(), gc.Equals, args.FromPort)
	c.Assert(pr.ToPort(), gc.Equals, args.ToPort)
	c.Assert(pr.Protocol(), gc.Equals, args.Protocol)
}

type OpenedPortsSerializationSuite struct {
	SliceSerializationSuite
	PortRangeCheck
}

var _ = gc.Suite(&OpenedPortsSerializationSuite{})

func (s *OpenedPortsSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "opened-ports"
	s.sliceName = "opened-ports"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importOpenedPorts(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["opened-ports"] = []interface{}{}
	}
}

func (s *OpenedPortsSerializationSuite) TestNewNetworkPorts(c *gc.C) {
	args := OpenedPortsArgs{
		SubnetID: "0.1.2.0/24",
		OpenedPorts: []PortRangeArgs{
			PortRangeArgs{
				UnitName: "magic/0",
				FromPort: 1234,
				ToPort:   2345,
				Protocol: "tcp",
			},
			PortRangeArgs{
				UnitName: "magic/0",
				FromPort: 1234,
				ToPort:   2345,
				Protocol: "udp",
			},
		},
	}

	ports := newOpenedPorts(args)
	c.Assert(ports.SubnetID(), gc.Equals, args.SubnetID)
	opened := ports.OpenPorts()
	c.Assert(opened, gc.HasLen, 2)
	s.AssertPortRange(c, opened[0], args.OpenedPorts[0])
	s.AssertPortRange(c, opened[1], args.OpenedPorts[1])
}

func (*OpenedPortsSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := &versionedOpenedPorts{
		Version: 1,
		OpenedPorts_: []*openedPorts{
			&openedPorts{
				SubnetID_: "fc00::/64",
				OpenedPorts_: &portRanges{
					Version: 1,
					OpenedPorts_: []*portRange{
						&portRange{
							UnitName_: "magic/0",
							FromPort_: 1234,
							ToPort_:   2345,
							Protocol_: "tcp",
						},
					},
				},
			},
			&openedPorts{
				SubnetID_: "192.168.0.0/16",
				OpenedPorts_: &portRanges{
					Version: 1,
					OpenedPorts_: []*portRange{
						&portRange{
							UnitName_: "unicorn/0",
							FromPort_: 80,
							ToPort_:   80,
							Protocol_: "tcp",
						},
					},
				},
			},
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	imported, err := importOpenedPorts(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(imported, jc.DeepEquals, initial.OpenedPorts_)
}

type PortRangeSerializationSuite struct {
	SliceSerializationSuite
	PortRangeCheck
}

var _ = gc.Suite(&PortRangeSerializationSuite{})

func (s *PortRangeSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "port-range"
	s.sliceName = "opened-ports"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importPortRanges(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["opened-ports"] = []interface{}{}
	}
}

func (s *PortRangeSerializationSuite) TestNewPortRange(c *gc.C) {
	args := PortRangeArgs{
		UnitName: "magic/0",
		FromPort: 1234,
		ToPort:   2345,
		Protocol: "tcp",
	}
	pr := newPortRange(args)
	s.AssertPortRange(c, pr, args)
}

func (*PortRangeSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := &portRanges{
		Version: 1,
		OpenedPorts_: []*portRange{
			&portRange{
				UnitName_: "magic/0",
				FromPort_: 1234,
				ToPort_:   2345,
				Protocol_: "tcp",
			},
			&portRange{
				UnitName_: "unicorn/1",
				FromPort_: 8080,
				ToPort_:   8080,
				Protocol_: "tcp",
			},
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	imported, err := importPortRanges(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(imported, jc.DeepEquals, initial.OpenedPorts_)
}
