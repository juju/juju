// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package description

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"
)

type ConstraintsSerializationSuite struct {
	SerializationSuite
}

var _ = gc.Suite(&ConstraintsSerializationSuite{})

func (s *ConstraintsSerializationSuite) SetUpTest(c *gc.C) {
	s.importName = "constraints"
	s.importFunc = func(m map[string]interface{}) (interface{}, error) {
		return importConstraints(m)
	}
}

func (s *ConstraintsSerializationSuite) allArgs() ConstraintsArgs {
	// NOTE: using gig from package_test.go
	return ConstraintsArgs{
		Architecture: "amd64",
		Container:    "lxd",
		CpuCores:     8,
		CpuPower:     4000,
		InstanceType: "magic",
		Memory:       16 * gig,
		RootDisk:     200 * gig,
		Spaces:       []string{"my", "own"},
		Tags:         []string{"much", "strong"},
	}
}

func (s *ConstraintsSerializationSuite) TestNewConstraints(c *gc.C) {
	args := s.allArgs()
	instance := newConstraints(args)

	c.Assert(instance.Architecture(), gc.Equals, args.Architecture)
	c.Assert(instance.Container(), gc.Equals, args.Container)
	c.Assert(instance.CpuCores(), gc.Equals, args.CpuCores)
	c.Assert(instance.CpuPower(), gc.Equals, args.CpuPower)
	c.Assert(instance.InstanceType(), gc.Equals, args.InstanceType)
	c.Assert(instance.Memory(), gc.Equals, args.Memory)
	c.Assert(instance.RootDisk(), gc.Equals, args.RootDisk)

	// Before we check tags and spaces, modify args to make sure that the
	// instance ones don't change.
	args.Spaces[0] = "weird"
	args.Tags[0] = "weird"
	spaces := instance.Spaces()
	c.Assert(spaces, jc.DeepEquals, []string{"my", "own"})
	tags := instance.Tags()
	c.Assert(tags, jc.DeepEquals, []string{"much", "strong"})

	// Also, changing the spaces tags returned, doesn't modify the instance
	spaces[0] = "weird"
	tags[0] = "weird"
	c.Assert(instance.Spaces(), jc.DeepEquals, []string{"my", "own"})
	c.Assert(instance.Tags(), jc.DeepEquals, []string{"much", "strong"})
}

func (s *ConstraintsSerializationSuite) TestNewConstraintsEmpty(c *gc.C) {
	instance := newConstraints(ConstraintsArgs{})
	c.Assert(instance, gc.IsNil)
}

func (s *ConstraintsSerializationSuite) TestEmptyTagsAndSpaces(c *gc.C) {
	instance := newConstraints(ConstraintsArgs{Architecture: "amd64"})
	// We actually want them to be nil, not empty slices.
	c.Assert(instance.Tags(), gc.IsNil)
	c.Assert(instance.Spaces(), gc.IsNil)
}

func (s *ConstraintsSerializationSuite) TestParsingSerializedData(c *gc.C) {
	initial := newConstraints(s.allArgs())
	bytes, err := yaml.Marshal(initial)
	c.Assert(err, jc.ErrorIsNil)

	var source map[string]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)

	instance, err := importConstraints(source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instance, jc.DeepEquals, initial)
}
