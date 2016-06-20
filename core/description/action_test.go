// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type ActionSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&ActionSerializationSuite{})

func (s *ActionSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "actions"
	s.sliceName = "actions"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importActions(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["actions"] = []interface{}{}
	}
}

func (s *ActionSerializationSuite) TestNewAction(c *gc.C) {
	args := ActionArgs{
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
	action := newAction(args)
	c.Assert(action.SubnetCIDR(), gc.Equals, args.SubnetCIDR)
	c.Assert(action.ProviderID(), gc.Equals, args.ProviderID)
	c.Assert(action.DeviceName(), gc.Equals, args.DeviceName)
	c.Assert(action.MachineID(), gc.Equals, args.MachineID)
	c.Assert(action.ConfigMethod(), gc.Equals, args.ConfigMethod)
	c.Assert(action.Value(), gc.Equals, args.Value)
	c.Assert(action.DNSServers(), jc.DeepEquals, args.DNSServers)
	c.Assert(action.DNSSearchDomains(), jc.DeepEquals, args.DNSSearchDomains)
	c.Assert(action.GatewayAddress(), gc.Equals, args.GatewayAddress)
}

func (s *ActionSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := actions{
		Version: 1,
		Actions_: []*action{
			newAction(ActionArgs{
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
			newAction(ActionArgs{Value: "10.0.0.5"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	actions, err := importActions(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(actions, jc.DeepEquals, initial.Actions_)
}
