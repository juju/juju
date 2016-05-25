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
		Name:       "special",
		Public:     true,
		ProviderID: "magic",
	}
	subnet := newSubnet(args)
	c.Assert(subnet.Name(), gc.Equals, args.Name)
	c.Assert(subnet.Public(), gc.Equals, args.Public)
	c.Assert(subnet.ProviderID(), gc.Equals, args.ProviderID)
}

func (s *SubnetSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := subnets{
		Version: 1,
		Subnets_: []*subnet{
			newSubnet(SubnetArgs{
				Name:       "special",
				Public:     true,
				ProviderID: "magic",
			}),
			newSubnet(SubnetArgs{Name: "foo"}),
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
