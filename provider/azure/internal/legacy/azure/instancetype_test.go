// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"strings"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/set"
	gc "gopkg.in/check.v1"
	"launchpad.net/gwacl"

	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/environs/testing"
)

type instanceTypeSuite struct {
	providerSuite
}

var _ = gc.Suite(&instanceTypeSuite{})

// setDummyStorage injects the local provider's fake storage implementation
// into the given environment, so that tests can manipulate storage as if it
// were real.
func (s *instanceTypeSuite) setDummyStorage(c *gc.C, env *azureEnviron) {
	closer, storage, _ := testing.CreateLocalTestStorage(c)
	env.storage = storage
	s.AddCleanup(func(c *gc.C) { closer.Close() })
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

func (s *instanceTypeSuite) TestSelectMachineTypeReturnsErrorIfNoMatch(c *gc.C) {
	var lots uint64 = 1000000000000
	env := s.setupEnvWithDummyMetadata(c)
	_, err := selectMachineType(env, constraints.Value{Mem: &lots})
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, `no instance types in West US matching constraints "mem=1000000000000M"`)
}

func (s *instanceTypeSuite) TestSelectMachineTypeReturnsCheapestMatch(c *gc.C) {
	var desiredCores uint64 = 50

	s.PatchValue(&getAvailableRoleSizes, func(*azureEnviron) (set.Strings, error) {
		return set.NewStrings("Panda", "LFA", "Lambo", "Veyron"), nil
	})

	costs := map[string]uint64{
		"Panda":  10,
		"LFA":    200,
		"Lambo":  100,
		"Veyron": 500,
	}
	s.PatchValue(&roleSizeCost, func(region, roleSize string) (uint64, error) {
		return costs[roleSize], nil
	})
	s.PatchValue(&gwacl.RoleSizes, []gwacl.RoleSize{
		// Cheap, but not up to our requirements.
		{Name: "Panda", CpuCores: desiredCores / 2},
		// Exactly what we need, but not the cheapest match.
		{Name: "LFA", CpuCores: desiredCores},
		// Much more power than we need, but actually cheaper.
		{Name: "Lambo", CpuCores: 2 * desiredCores},
		// Way out of our league.
		{Name: "Veyron", CpuCores: 10 * desiredCores},
	})

	env := s.setupEnvWithDummyMetadata(c)
	choice, err := selectMachineType(env, constraints.Value{CpuCores: &desiredCores})
	c.Assert(err, jc.ErrorIsNil)

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
	s.makeTestMetadata(c, "precise", "West US", images)
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
	c.Assert(err, jc.ErrorIsNil)
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
	s.makeTestMetadata(c, "precise", "West US", images)
	images, err := findMatchingImages(env, "West US", "precise", []string{"amd64"})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(images, gc.HasLen, 1)
	c.Assert(images[0].Id, gc.Equals, "image-id")
}

func (s *instanceTypeSuite) TestNewInstanceTypeConvertsRoleSize(c *gc.C) {
	const expectedRegion = "expected"
	s.PatchValue(&roleSizeCost, func(region, roleSize string) (uint64, error) {
		c.Assert(region, gc.Equals, expectedRegion)
		return 999999500, nil
	})
	roleSize := gwacl.RoleSize{
		Name:          "Outrageous",
		CpuCores:      128,
		Mem:           4 * gwacl.TB,
		OSDiskSpace:   48 * gwacl.TB,
		TempDiskSpace: 50 * gwacl.TB,
		MaxDataDisks:  20,
	}
	vtype := "Hyper-V"
	expectation := instances.InstanceType{
		Id:       roleSize.Name,
		Name:     roleSize.Name,
		CpuCores: roleSize.CpuCores,
		Mem:      roleSize.Mem,
		RootDisk: roleSize.OSDiskSpace,
		Cost:     999999500,
		VirtType: &vtype,
	}
	instType, err := newInstanceType(roleSize, expectedRegion)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instType, gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestListInstanceTypesAGVNetRoleSizeFiltering(c *gc.C) {
	// Old environments with a virtual network tied to an affinity group
	// will cause D and G series to be filtered out.
	expectation := make([]instances.InstanceType, 0, len(gwacl.RoleSizes))
	for _, roleSize := range gwacl.RoleSizes {
		if strings.HasPrefix(roleSize.Name, "Basic_") {
			continue
		}
		if strings.HasPrefix(roleSize.Name, "Standard_") {
			continue
		}
		instanceType, err := newInstanceType(roleSize, "West US")
		c.Assert(err, jc.ErrorIsNil)
		instanceType.Arches = []string{"amd64"}
		expectation = append(expectation, instanceType)
	}

	s.PatchValue(&getVirtualNetwork, func(*azureEnviron) (*gwacl.VirtualNetworkSite, error) {
		return &gwacl.VirtualNetworkSite{Name: "vnet", AffinityGroup: "ag"}, nil
	})
	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestListInstanceTypesNoVNetNoRoleSizeFiltering(c *gc.C) {
	// If there's no virtual network yet, we'll create one with a
	// location rather than an affinity group; thus we do not limit
	// which instance types are available.
	expectation := make([]instances.InstanceType, 0, len(gwacl.RoleSizes))
	for _, roleSize := range gwacl.RoleSizes {
		if strings.HasPrefix(roleSize.Name, "Basic_") {
			continue
		}
		instanceType, err := newInstanceType(roleSize, "West US")
		c.Assert(err, jc.ErrorIsNil)
		instanceType.Arches = []string{"amd64"}
		expectation = append(expectation, instanceType)
	}

	s.PatchValue(&getVirtualNetwork, func(*azureEnviron) (*gwacl.VirtualNetworkSite, error) {
		return nil, errors.NotFoundf("virtual network")
	})
	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestListInstanceTypesLocationFiltering(c *gc.C) {
	available := set.NewStrings("Standard_D1")
	s.PatchValue(&getAvailableRoleSizes, func(*azureEnviron) (set.Strings, error) {
		return available, nil
	})

	// If there's no virtual network yet, we'll create one with a
	// location rather than an affinity group; thus we do not limit
	// which instance types are available.
	expectation := make([]instances.InstanceType, 0, len(gwacl.RoleSizes))
	for _, roleSize := range gwacl.RoleSizes {
		if !available.Contains(roleSize.Name) {
			continue
		}
		instanceType, err := newInstanceType(roleSize, "West US")
		c.Assert(err, jc.ErrorIsNil)
		instanceType.Arches = []string{"amd64"}
		expectation = append(expectation, instanceType)
	}

	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestListInstanceTypesMaintainsOrder(c *gc.C) {
	expectation := make([]instances.InstanceType, 0, len(gwacl.RoleSizes))
	for _, roleSize := range gwacl.RoleSizes {
		if strings.HasPrefix(roleSize.Name, "Basic_") {
			continue
		}
		instanceType, err := newInstanceType(roleSize, "West US")
		c.Assert(err, jc.ErrorIsNil)
		instanceType.Arches = []string{"amd64"}
		expectation = append(expectation, instanceType)
	}

	env := s.setupEnvWithDummyMetadata(c)
	types, err := listInstanceTypes(env)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(types, gc.DeepEquals, expectation)
}

func (s *instanceTypeSuite) TestFindInstanceSpecFailsImpossibleRequest(c *gc.C) {
	impossibleConstraint := &instances.InstanceConstraint{
		Series: "precise",
		Arches: []string{"axp"},
	}

	env := s.setupEnvWithDummyMetadata(c)
	_, err := findInstanceSpec(env, impossibleConstraint)
	c.Assert(err, gc.NotNil)
	c.Check(err, gc.ErrorMatches, "no OS images found for .*")
}

var findInstanceSpecTests = []struct {
	series string
	cons   string
	itype  string
}{
	{
		series: "precise",
		cons:   "mem=7G cpu-cores=2",
		itype:  "Standard_D2",
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
		c.Assert(err, jc.ErrorIsNil)

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
	aim := roleSizeByName("Large")
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
	c.Assert(err, jc.ErrorIsNil)

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
	c.Assert(err, jc.ErrorIsNil)

	c.Check(spec.InstanceType.Name, gc.Equals, "Small")
}

func (s *instanceTypeSuite) TestPrecheckInstanceValidInstanceType(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	cons := constraints.MustParse("instance-type=Large")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *instanceTypeSuite) TestPrecheckInstanceInvalidInstanceType(c *gc.C) {
	env := s.setupEnvWithDummyMetadata(c)
	cons := constraints.MustParse("instance-type=Super")
	placement := ""
	err := env.PrecheckInstance("precise", cons, placement)
	c.Assert(err, gc.ErrorMatches, `invalid instance type "Super"`)
}
