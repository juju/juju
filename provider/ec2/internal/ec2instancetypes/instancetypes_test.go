// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package ec2instancetypes_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/provider/ec2/internal/ec2instancetypes"
)

type InstanceTypesSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&InstanceTypesSuite{})

func (s *InstanceTypesSuite) TestRegionInstanceTypes(c *gc.C) {
	// This is the set of instance type names we had hard coded previously.
	knownInstanceTypes := set.NewStrings(
		"m1.small", "m1.medium", "m1.large", "m1.xlarge",
		"m4.large", "m4.xlarge", "m4.2xlarge", "m4.4xlarge", "m4.10xlarge",
		"m3.medium", "m3.large", "m3.xlarge", "m3.2xlarge",
		"c1.medium", "c1.xlarge", "cc2.8xlarge",
		"c3.large", "c3.xlarge", "c3.2xlarge", "c3.4xlarge", "c3.8xlarge",
		"cg1.4xlarge",
		"g2.2xlarge",
		"m2.xlarge", "m2.2xlarge", "m2.4xlarge", "cr1.8xlarge",
		"r3.large", "r3.xlarge", "r3.2xlarge", "r3.4xlarge", "r3.8xlarge",
		"hi1.4xlarge",
		"i2.xlarge", "i2.2xlarge", "i2.8xlarge", "hs1.8xlarge",
		"t1.micro",
		"t2.micro", "t2.small", "t2.medium",
		"c4.large", "c4.xlarge", "c4.2xlarge", "c4.4xlarge", "c4.8xlarge",
	)
	seen := make(map[string]bool)
	var unknownInstanceTypes []string
	instanceTypes := ec2instancetypes.RegionInstanceTypes("us-east-1")
	for _, instanceType := range instanceTypes {
		c.Assert(instanceType.Cost, gc.Not(gc.Equals), 0)
		c.Assert(seen[instanceType.Name], jc.IsFalse) // no duplicates
		seen[instanceType.Name] = true

		if !knownInstanceTypes.Contains(instanceType.Name) {
			unknownInstanceTypes = append(unknownInstanceTypes, instanceType.Name)
		} else {
			knownInstanceTypes.Remove(instanceType.Name)
		}
	}
	c.Assert(knownInstanceTypes, gc.HasLen, 0) // all accounted for
	if len(unknownInstanceTypes) > 0 {
		c.Logf("unknown instance types: %s", unknownInstanceTypes)
	}
}

func (s *InstanceTypesSuite) TestRegionInstanceTypesAvailability(c *gc.C) {
	// Some instance types are only available in some regions.
	usWest1InstanceTypes := set.NewStrings()
	usEast1InstanceTypes := set.NewStrings()
	for _, instanceType := range ec2instancetypes.RegionInstanceTypes("us-west-1") {
		usWest1InstanceTypes.Add(instanceType.Name)
	}
	for _, instanceType := range ec2instancetypes.RegionInstanceTypes("us-east-1") {
		usEast1InstanceTypes.Add(instanceType.Name)
	}
	c.Assert(
		usEast1InstanceTypes.Difference(usWest1InstanceTypes).SortedValues(),
		jc.DeepEquals,
		[]string{
			"cc2.8xlarge", "cg1.4xlarge", "cr1.8xlarge", "f1.16xlarge",
			"f1.2xlarge", "hi1.4xlarge", "hs1.8xlarge", "p2.16xlarge",
			"p2.8xlarge", "p2.xlarge", "x1.16xlarge", "x1.32xlarge",
		},
	)
}

func (s *InstanceTypesSuite) TestRegionInstanceTypesUnknownRegion(c *gc.C) {
	instanceTypes := ec2instancetypes.RegionInstanceTypes("cn-north-1")
	c.Assert(instanceTypes, jc.DeepEquals, ec2instancetypes.RegionInstanceTypes("us-east-1"))
}

func (s *InstanceTypesSuite) TestSupportsClassic(c *gc.C) {
	assertSupportsClassic := func(name string) {
		c.Assert(ec2instancetypes.SupportsClassic(name), jc.IsTrue)
	}
	assertDoesNotSupportClassic := func(name string) {
		c.Assert(ec2instancetypes.SupportsClassic(name), jc.IsFalse)
	}
	assertSupportsClassic("c1.medium")
	assertSupportsClassic("c3.large")
	assertSupportsClassic("cc2.8xlarge")
	assertSupportsClassic("cg1.4xlarge")
	assertSupportsClassic("cr1.8xlarge")
	assertSupportsClassic("d2.8xlarge")
	assertSupportsClassic("g2.2xlarge")
	assertSupportsClassic("hi1.4xlarge")
	assertSupportsClassic("hs1.8xlarge")
	assertSupportsClassic("i2.2xlarge")
	assertSupportsClassic("m1.medium")
	assertSupportsClassic("m2.medium")
	assertSupportsClassic("m3.medium")
	assertSupportsClassic("r3.8xlarge")
	assertSupportsClassic("t1.micro")
	assertDoesNotSupportClassic("c4.large")
	assertDoesNotSupportClassic("m4.large")
	assertDoesNotSupportClassic("p2.xlarge")
	assertDoesNotSupportClassic("t2.medium")
	assertDoesNotSupportClassic("x1.32xlarge")
}
