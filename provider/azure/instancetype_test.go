// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/gwacl"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/environs/testing"
)

type instanceTypeSuite struct {
	providerSuite
}

var _ = gc.Suite(&instanceTypeSuite{})

func (s *instanceTypeSuite) SetUpTest(c *gc.C) {
	s.providerSuite.SetUpTest(c)
	s.PatchValue(&imagemetadata.DefaultBaseURL, "")
	s.PatchValue(&signedImageDataOnly, false)
}

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *instanceTypeSuite) setDummyStorage(c *gc.C, env *azureEnviron) {
	closer, storage, _ := testing.CreateLocalTestStorage(c)
	env.storage = storage
	s.AddCleanup(func(c *gc.C) { closer.Close() })
}

func (*instanceTypeSuite) TestNewPreferredTypesAcceptsNil(c *gc.C) {
	types := newPreferredTypes(nil)

	c.Check(types, gc.HasLen, 0)
	c.Check(types.Len(), gc.Equals, 0)
}

func (*instanceTypeSuite) TestNewPreferredTypesRepresentsInput(c *gc.C) {
	availableTypes := []gwacl.RoleSize{{Name: "Humongous", Cost: 123}}

	types := newPreferredTypes(availableTypes)

	c.Assert(types, gc.HasLen, len(availableTypes))
	c.Check(types[0], gc.Equals, &availableTypes[0])
	c.Check(types.Len(), gc.Equals, len(availableTypes))
}

func (*instanceTypeSuite) TestNewPreferredTypesSortsByCost(c *gc.C) {
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

func (*instanceTypeSuite) TestLessComparesCost(c *gc.C) {
	types := preferredTypes{
		{Name: "Cheap", Cost: 1},
		{Name: "Posh", Cost: 200},
	}

	c.Check(types.Less(0, 1), gc.Equals, true)
	c.Check(types.Less(1, 0), gc.Equals, false)
}

func (*instanceTypeSuite) TestSwapSwitchesEntries(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(0, 1)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*instanceTypeSuite) TestSwapIsCommutative(c *gc.C) {
	types := preferredTypes{
		{Name: "First"},
		{Name: "Last"},
	}

	types.Swap(1, 0)

	c.Check(types[0].Name, gc.Equals, "Last")
	c.Check(types[1].Name, gc.Equals, "First")
}

func (*instanceTypeSuite) TestSwapLeavesOtherEntriesIntact(c *gc.C) {
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

func (*instanceTypeSuite) TestSufficesAcceptsNilRequirement(c *gc.C) {
	types := preferredTypes{}
	c.Check(types.suffices(0, nil), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesAcceptsMetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesAcceptsExactRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation+1, &expectation), gc.Equals, true)
}

func (*instanceTypeSuite) TestSufficesRejectsUnmetRequirement(c *gc.C) {
	types := preferredTypes{}
	var expectation uint64 = 100
	c.Check(types.suffices(expectation-1, &expectation), gc.Equals, false)
}

func (*instanceTypeSuite) TestSatisfiesComparesCPUCores(c *gc.C) {
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

func (*instanceTypeSuite) TestSatisfiesComparesMem(c *gc.C) {
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

func (*instanceTypeSuite) TestDefaultToBaselineSpecSetsMimimumMem(c *gc.C) {
	c.Check(
		*defaultToBaselineSpec(constraints.Value{}).Mem,
		gc.Equals,
		uint64(defaultMem))
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesOriginalIntact(c *gc.C) {
	original := constraints.Value{}
	defaultToBaselineSpec(original)
	c.Check(original.Mem, gc.IsNil)
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesLowerMemIntact(c *gc.C) {
	const low = 100 * gwacl.MB
	var value uint64 = low
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(low))
}

func (*instanceTypeSuite) TestDefaultToBaselineSpecLeavesHigherMemIntact(c *gc.C) {
	const high = 100 * gwacl.MB
	var value uint64 = high
	c.Check(
		defaultToBaselineSpec(constraints.Value{Mem: &value}).Mem,
		gc.Equals,
		&value)
	c.Check(value, gc.Equals, uint64(high))
}

func (*instanceTypeSuite) TestSelectMachineTypeReturnsErrorIfNoMatch(c *gc.C) {
	var lots uint64 = 1000000000000
	_, err := selectMachineType(nil, constraints.Value{Mem: &lots})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no machine type matches constraints mem=100000*[MGT]")
}

func (*instanceTypeSuite) TestSelectMachineTypeReturnsCheapestMatch(c *gc.C) {
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

func (s *instanceTypeSuite) setupEnvWithDummyMetadata(c *gc.C) *azureEnviron {
	envAttrs := makeAzureConfigMap(c)
	envAttrs["location"] = "West US"
	env := makeEnvironWithConfig(c, envAttrs)
	s.setDummyStorage(c, env)
	images := []*imagemetadata.ImageMetadata{
		{
			Id:         "image-id",
			VirtType:   "Hyper-V",
			Arch:       "amd64",
			RegionName: "West US",
			Endpoint:   "https://management.core.windows.net/",
		},
	}
	makeTestMetadata(c, env, "precise", "West US", images)
	return env
}

func (s *instanceTypeSuite) TestFindMatchingImagesReturnsErrorIfNoneFound(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	_, err := findMatchingImages(env, "West US", "saucy", []string{"amd64"})
	c.Assert(err, gc.NotNil)
	c.Assert(err, gc.ErrorMatches, "no OS images found for location .*")
}

func (s *instanceTypeSuite) TestFindMatchingImagesReturnsReleasedImages(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	images, err := findMatchingImages(env, "West US", "precise", []string{"amd64"})
	c.Assert(err, gc.IsNil)
	c.Assert(images, gc.HasLen, 1)
	c.Check(images[0].Id, gc.Equals, "image-id")
}

func (s *instanceTypeSuite) TestFindMatchingImagesReturnsDailyImages(c *gc.C) {
	envAttrs := makeAzureConfigMap(c)
	envAttrs["image-stream"] = "daily"
	envAttrs["location"] = "West US"
	env := makeEnvironWithConfig(c, envAttrs)
	s.setDummyStorage(c, env)
	images := []*imagemetadata.ImageMetadata{
		{
			Id:         "image-id",
			VirtType:   "Hyper-V",
			Arch:       "amd64",
			RegionName: "West US",
			Endpoint:   "https://management.core.windows.net/",
			Stream:     "daily",
		},
	}
	makeTestMetadata(c, env, "precise", "West US", images)
	images, err := findMatchingImages(env, "West US", "precise", []string{"amd64"})
	c.Assert(err, gc.IsNil)
	c.Assert(images, gc.HasLen, 1)
	c.Assert(images[0].Id, gc.Equals, "image-id")
}

func (*instanceTypeSuite) TestNewInstanceTypeConvertsRoleSize(c *gc.C) {
	roleSize := gwacl.RoleSize{
		Name:             "Outrageous",
		CpuCores:         128,
		Mem:              4 * gwacl.TB,
		OSDiskSpaceCloud: 48 * gwacl.TB,
		OSDiskSpaceVirt:  50 * gwacl.TB,
		MaxDataDisks:     20,
		Cost:             999999500,
	}
	vtype := "Hyper-V"
	var cpupower uint64 = 100
	expectation := instances.InstanceType{
		Id:       roleSize.Name,
		Name:     roleSize.Name,
		CpuCores: roleSize.CpuCores,
		Mem:      roleSize.Mem,
		RootDisk: roleSize.OSDiskSpaceVirt,
		Cost:     roleSize.Cost,
		VirtType: &vtype,
		CpuPower: &cpupower,
	}
	c.Assert(newInstanceType(roleSize), gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestListInstanceTypesAcceptsNil(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env, nil)
	c.Assert(err, gc.IsNil)
	c.Check(types, gc.HasLen, 0)
}

func (s *instanceTypeSuite) TestListInstanceTypesMaintainsOrder(c *gc.C) {
	roleSizes := []gwacl.RoleSize{
		{Name: "Biggish"},
		{Name: "Tiny"},
		{Name: "Huge"},
		{Name: "Miniscule"},
	}

	expectation := make([]instances.InstanceType, len(roleSizes))
	for index, roleSize := range roleSizes {
		expectation[index] = newInstanceType(roleSize)
		expectation[index].Arches = []string{"amd64"}
	}

	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env, roleSizes)
	c.Assert(err, gc.IsNil)
	c.Assert(types, gc.DeepEquals, expectation)
}

func (*instanceTypeSuite) TestFindInstanceSpecFailsImpossibleRequest(c *gc.C) {
	impossibleConstraint := &instances.InstanceConstraint{
		Series: "precise",
		Arches: []string{"axp"},
	}

	env := makeEnviron(c)
	_, err := findInstanceSpec(env, impossibleConstraint)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no OS images found for .*")
}

func makeTestMetadata(c *gc.C, env environs.Environ, series, location string, im []*imagemetadata.ImageMetadata) {
	cloudSpec := simplestreams.CloudSpec{
		Region:   location,
		Endpoint: "https://management.core.windows.net/",
	}
	err := imagemetadata.MergeAndWriteMetadata(series, im, &cloudSpec, env.Storage())
	c.Assert(err, gc.IsNil)
}

var findInstanceSpecTests = []struct {
	series string
	cons   string
	itype  string
}{
	{
		series: "precise",
		cons:   "mem=7G cpu-cores=2",
		itype:  "Large",
	}, {
		series: "precise",
		cons:   "instance-type=ExtraLarge",
	},
}

func (s *instanceTypeSuite) TestFindInstanceSpec(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	for i, t := range findInstanceSpecTests {
		c.Logf("test %d", i)

		cons := constraints.MustParse(t.cons)
		constraints := &instances.InstanceConstraint{
			Region:      "West US",
			Series:      t.series,
			Arches:      []string{"amd64"},
			Constraints: cons,
		}

		// Find a matching instance type and image.
		spec, err := findInstanceSpec(env, constraints)
		c.Assert(err, gc.IsNil)

		// We got the instance type we described in our constraints, and
		// the image returned by (the fake) simplestreams.
		if cons.HasInstanceType() {
			c.Check(spec.InstanceType.Name, gc.Equals, *cons.InstanceType)
		} else {
			c.Check(spec.InstanceType.Name, gc.Equals, t.itype)
		}
		c.Check(spec.Image.Id, gc.Equals, "image-id")
	}
}

func (s *instanceTypeSuite) TestFindInstanceSpecFindsMatch(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)

	// We'll tailor our constraints to describe one particular Azure
	// instance type:
	aim := gwacl.RoleNameMap["Large"]
	constraints := &instances.InstanceConstraint{
		Region: "West US",
		Series: "precise",
		Arches: []string{"amd64"},
		Constraints: constraints.Value{
			CpuCores: &aim.CpuCores,
			Mem:      &aim.Mem,
		},
	}

	// Find a matching instance type and image.
	spec, err := findInstanceSpec(env, constraints)
	c.Assert(err, gc.IsNil)

	// We got the instance type we described in our constraints, and
	// the image returned by (the fake) simplestreams.
	c.Check(spec.InstanceType.Name, gc.Equals, aim.Name)
	c.Check(spec.Image.Id, gc.Equals, "image-id")
}

func (s *instanceTypeSuite) TestFindInstanceSpecSetsBaseline(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)

	// findInstanceSpec sets baseline constraints, so that it won't pick
	// ExtraSmall (which is too small for routine tasks) if you fail to
	// set sufficient hardware constraints.
	anyInstanceType := &instances.InstanceConstraint{
		Region: "West US",
		Series: "precise",
		Arches: []string{"amd64"},
	}

	spec, err := findInstanceSpec(env, anyInstanceType)
	c.Assert(err, gc.IsNil)

	c.Check(spec.InstanceType.Name, gc.Equals, "Small")
}

func (s *instanceTypeSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	cons := constraints.MustParse("instance-type=Large")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, gc.IsNil)
}

func (s *instanceTypeSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	cons := constraints.MustParse("instance-type=Super")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, gc.ErrorMatches, `invalid Azure instance "Super" specified`)
}
