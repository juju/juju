// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
)

type InstanceTypeSuite struct{}

var _ = gc.Suite(&InstanceTypeSuite{})

func (*InstanceTypeSuite) TestNewPreferredTypesAcceptsNil(c *gc.C) {
	types := newPreferredTypes(nil)

	c.Check(types, gc.HasLen, 0)
	c.Check(types.Len(), gc.Equals, 0)
}

func (*InstanceTypeSuite) TestNewPreferredTypesRepresentsInput(c *gc.C) {
	availableTypes := []gwacl.RoleSize{{Name: "Humongous", Cost: 123}}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	c.Check(types[0], gc.Equals, &availableTypes[0])
	c.Check(types.Len(), gc.Equals, len(availableTypes))
}

func (*InstanceTypeSuite) TestNewPreferredTypesSortsByCost(c *gc.C) {
	availableTypes := []gwacl.RoleSize{
		{Name: "Excessive", Cost: 12},
		{Name: "Ridiculous", Cost: 99},
		{Name: "Modest", Cost: 3},
	}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	// We end up with machine types sorted by ascending cost.
	c.Check(types[0].Name, gc.Equals, "Modest")
	c.Check(types[1].Name, gc.Equals, "Excessive")
	c.Check(types[2].Name, gc.Equals, "Ridiculous")
}

func (*InstanceTypeSuite) TestLessComparesCost(c *gc.C) {
	types := preferredTypes{
		{Name: "Cheap", Cost: 1},
		{Name: "Posh", Cost: 200},
	}

	c.Check(types.Less(0, 1), gc.Equals, true)
	c.Check(types.Less(1, 0), gc.Equals, false)
}

func (*InstanceTypeSuite) TestSwapSwitchesEntries(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(0, 1)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*InstanceTypeSuite) TestSwapIsCommutative(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(1, 0)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*InstanceTypeSuite) TestSwapLeavesOtherEntriesIntact(c *gc.C) {
	types := preferredTypes{
		{Name: "A"},
		{Name: "B"},
		{Name: "C"},
		{Name: "D"},
	}

	types.Swap(1, 2)

	c.Check(types[0].Name, gc.Equals, "A")
	c.Check(types[1].Name, gc.Equals, "C")
	c.Check(types[2].Name, gc.Equals, "B")
	c.Check(types[3].Name, gc.Equals, "D")
}

func (*InstanceTypeSuite) TestSufficesAcceptsNilRequirement(c *gc.C) {
	types := preferredTypes{}
	c.Check(types.suffices(0, nil), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesAcceptsMetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesAcceptsExactRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSufficesRejectsUnmetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation-1, &expectation), gc.Equals, false)
}

func (*InstanceTypeSuite) TestSatisfiesComparesCPUCores(c *gc.C) {
	types := preferredTypes{}
	var desiredCores uint64 = 5
	constraint := constraints.Value{CpuCores: &desiredCores}

	// A machine with fewer cores than required does not satisfy...
	machine := gwacl.RoleSize{CpuCores: desiredCores - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more cores.
	machine.CpuCores = desiredCores
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*InstanceTypeSuite) TestSatisfiesComparesMem(c *gc.C) {
	types := preferredTypes{}
	var desiredMem uint64 = 37
	constraint := constraints.Value{Mem: &desiredMem}

	// A machine with less memory than required does not satisfy...
	machine := gwacl.RoleSize{Mem: desiredMem - 1}
	c.Check(types.satisfies(&machine, constraint), gc.Equals, false)
	// ...Even if it would, given more memory.
	machine.Mem = desiredMem
	c.Check(types.satisfies(&machine, constraint), gc.Equals, true)
}

func (*InstanceTypeSuite) TestIsValidArch(c *gc.C) {
	types := preferredTypes{}

	// No architecture needs to be specified.
	c.Check(types.isValidArch(nil), gc.Equals, true)

	// Azure supports these architectures...
	supported := []string{
		"amd64",
		"i386",
	}
	for _, arch := range supported {
		c.Log("Checking that %q is supported.", arch)
		c.Check(types.isValidArch(&arch), gc.Equals, true)
	}

	// ...But not these.
	unsupported := []string{
		"",
		"axp",
		"powerpc",
	}
	for _, arch := range unsupported {
		c.Log("Checking that %q is not supported.", arch)
		c.Check(types.isValidArch(&arch), gc.Equals, false)
	}
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecSetsMimimumMem(c *gc.C) {
	c.Check(
		*defaultToBaselineSpec(constraints.Value{}).Mem,
		gc.Equals,
		uint64(defaultMem))
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesOriginalIntact(c *gc.C) {
	original := constraints.Value{}
	defaultToBaselineSpec(original)
	c.Check(original.Mem, gc.IsNil)
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesLowerMemIntact(c *gc.C) {
	const low = 100 * gwacl.MB
	var value uint64 = low
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(low))
}

func (*InstanceTypeSuite) TestDefaultToBaselineSpecLeavesHigherMemIntact(c *gc.C) {
	const high = 100 * gwacl.MB
	var value uint64 = high
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(high))
}

func (*InstanceTypeSuite) TestSelectMachineTypeChecksArch(c *gc.C) {
	unsupportedArch := "amd32k"
	constraint := constraints.Value{Arch: &unsupportedArch}
	_, err := selectMachineType(nil, constraint)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, `requested unsupported architecture "amd32k"`)
}

func (*InstanceTypeSuite) TestSelectMachineTypeReturnsErrorIfNoMatch(c *gc.C) {
	var lots uint64 = 1000000000000
	_, err := selectMachineType(nil, constraints.Value{Mem: &lots})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no machine type matches constraints mem=100000*[MGT]")
}

func (*InstanceTypeSuite) TestSelectMachineTypeReturnsCheapestMatch(c *gc.C) {
	var desiredCores uint64 = 50
	availableTypes := []gwacl.RoleSize{
		// Cheap, but not up to our requirements.
		{Name: "Panda", CpuCores: desiredCores / 2, Cost: 10},
		// Exactly what we need, but not the cheapest match.
		{Name: "LFA", CpuCores: desiredCores, Cost: 200},
		// Much more power than we need, but actually cheaper.
		{Name: "Lambo", CpuCores: 2 * desiredCores, Cost: 100},
		// Way out of our league.
		{Name: "Veyron", CpuCores: 10 * desiredCores, Cost: 500},
	}

	choice, err := selectMachineType(availableTypes, constraints.Value{CpuCores: &desiredCores})
	c.Assert(err, gc.IsNil)

	// Out of these options, selectMachineType picks not the first; not
	// the cheapest; not the biggest; not the last; but the cheapest type
	// of machine that meets requirements.
	c.Check(choice.Name, gc.Equals, "Lambo")
}

func (*InstanceTypeSuite) TestGetImageBaseURLs(c *gc.C) {
	env := makeEnviron(c)
	urls, err := env.getImageBaseURLs()
	c.Assert(err, gc.IsNil)
	// At the moment this is not configurable.  It returns a fixed URL for
	// the central simplestreams database.
	c.Check(urls, gc.DeepEquals, []string{imagemetadata.DefaultBaseURL})
}

func (*InstanceTypeSuite) TestGetEndpointReturnsFixedEndpointForSupportedRegion(c *gc.C) {
	env := makeEnviron(c)
	endpoint, err := env.getEndpoint("West US")
	c.Assert(err, gc.IsNil)
	c.Check(endpoint, gc.Equals, "https://management.core.windows.net/")
}

func (*InstanceTypeSuite) TestGetEndpointReturnsChineseEndpointForChina(c *gc.C) {
	env := makeEnviron(c)
	endpoint, err := env.getEndpoint("China East")
	c.Assert(err, gc.IsNil)
	c.Check(endpoint, gc.Equals, "https://management.core.chinacloudapi.cn/")
}

func (*InstanceTypeSuite) TestGetEndpointRejectsUnknownRegion(c *gc.C) {
	region := "Central South San Marino Highlands"
	env := makeEnviron(c)
	endpoint, err := env.getEndpoint(region)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "unknown region: "+region)
}

func (*InstanceTypeSuite) TestGetImageStreamDefaultsToBlank(c *gc.C) {
	env := makeEnviron(c)
	// Hard-coded to default for now.
	c.Check(env.getImageStream(), gc.Equals, "")
}

func (*InstanceTypeSuite) TestGetImageSigningRequiredDefaultsToTrue(c *gc.C) {
	env := makeEnviron(c)
	// Hard-coded to true for now.
	c.Check(env.getImageSigningRequired(), gc.Equals, true)
}
