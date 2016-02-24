// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migration

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

func minimalConstraintsMap() map[interface{}]interface{} {
	return map[interface{}]interface{}{
		"version": 1,
	}
}

func minimalConstraints() *constraints {
	return newConstraints(minimalConstraintsArgs())
}

func minimalConstraintsArgs() ConstraintsArgs {
	return ConstraintsArgs{}
}

func (s *ConstraintsSerializationSuite) allArgs() ConstraintsArgs {
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
	// NOTE: using gig from package_test.go
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

func (s *ConstraintsSerializationSuite) TestMinimalMatches(c *gc.C) {
	bytes, err := yaml.Marshal(minimalConstraints())
	c.Assert(err, jc.ErrorIsNil)

	var source map[interface{}]interface{}
	err = yaml.Unmarshal(bytes, &source)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(source, jc.DeepEquals, minimalConstraintsMap())
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
