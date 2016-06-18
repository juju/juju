// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type SpaceSerializationSuite struct {
	SliceSerializationSuite
}

var _ = gc.Suite(&SpaceSerializationSuite{})

func (s *SpaceSerializationSuite) SetUpTest(c *gc.C) {
	s.SliceSerializationSuite.SetUpTest(c)
	s.importName = "spaces"
	s.sliceName = "spaces"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importSpaces(m)
	}
	s.testFields = func(m map[string]interface{}) {
		m["spaces"] = []interface{}{}
	}
}

func (s *SpaceSerializationSuite) TestNewSpace(c *gc.C) {
	args := SpaceArgs{
		Name:       "special",
		Public:     true,
		ProviderID: "magic",
	}
	space := newSpace(args)
	c.Assert(space.Name(), gc.Equals, args.Name)
	c.Assert(space.Public(), gc.Equals, args.Public)
	c.Assert(space.ProviderID(), gc.Equals, args.ProviderID)
}

func (s *SpaceSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := spaces{
		Version: 1,
		Spaces_: []*space{
			newSpace(SpaceArgs{
				Name:       "special",
				Public:     true,
				ProviderID: "magic",
			}),
			newSpace(SpaceArgs{Name: "foo"}),
		},
	}

	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	spaces, err := importSpaces(source)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(spaces, jc.DeepEquals, initial.Spaces_)
}
