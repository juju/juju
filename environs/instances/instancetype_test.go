// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"sort"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/testing"
)

type instanceTypeSuite struct {
	testing.BaseSuite
}

func TestInstanceTypeSuite(t *stdtesting.T) { tc.Run(t, &instanceTypeSuite{}) }

var hvm = "hvm"

// The instance types below do not necessarily reflect reality and are just
// defined here for ease of testing special cases.
var instanceTypes = []InstanceType{
	{
		Name:     "m1.small",
		Arch:     "amd64",
		CpuCores: 1,
		CpuPower: CpuPower(100),
		Mem:      1740,
		Cost:     60,
		RootDisk: 8192,
	}, {
		Name:     "m1.medium",
		Arch:     "amd64",
		CpuCores: 1,
		CpuPower: CpuPower(200),
		Mem:      3840,
		Cost:     120,
		RootDisk: 16384,
	}, {
		Name:     "m1.large",
		Arch:     "amd64",
		CpuCores: 2,
		CpuPower: CpuPower(400),
		Mem:      7680,
		Cost:     240,
		RootDisk: 32768,
	}, {
		Name:     "m1.xlarge",
		Arch:     "amd64",
		CpuCores: 4,
		CpuPower: CpuPower(800),
		Mem:      15360,
		Cost:     480,
	}, {
		Name:     "t1.micro",
		Arch:     "amd64",
		CpuCores: 1,
		CpuPower: CpuPower(20),
		Mem:      613,
		Cost:     20,
		RootDisk: 4096,
	}, {
		Name:     "c1.medium",
		Arch:     "amd64",
		CpuCores: 2,
		CpuPower: CpuPower(500),
		Mem:      1740,
		Cost:     145,
		RootDisk: 8192,
	}, {
		Name:     "c1.large",
		Arch:     "arm64",
		CpuCores: 2,
		CpuPower: CpuPower(500),
		Mem:      3264,
		Cost:     520,
		RootDisk: 8192,
	}, {
		Name:     "c1.xlarge",
		Arch:     "amd64",
		CpuCores: 8,
		CpuPower: CpuPower(2000),
		Mem:      7168,
		Cost:     580,
	}, {
		Name:     "cc1.4xlarge",
		Arch:     "amd64",
		CpuCores: 8,
		CpuPower: CpuPower(3350),
		Mem:      23552,
		Cost:     1300,
		VirtType: &hvm,
	}, {
		Name:     "cc2.8xlarge",
		Arch:     "amd64",
		CpuCores: 16,
		CpuPower: CpuPower(8800),
		Mem:      61952,
		Cost:     2400,
		VirtType: &hvm,
	}, {
		Name:        "VM.Standard3.Flex",
		Arch:        "amd64",
		CpuCores:    1,
		CpuPower:    CpuPower(8800),
		MaxCpuCores: makeUint64Pointer(32),
		Mem:         6144,
		MaxMem:      makeUint64Pointer(524288),
		Cost:        50,
	},
}

var getInstanceTypesTest = []struct {
	about          string
	cons           string
	itypesToUse    []InstanceType
	expectedItypes []string
	arch           string
}{
	{
		about: "cores",
		cons:  "cores=2",
		expectedItypes: []string{
			"VM.Standard3.Flex", "m1.large", "m1.xlarge", "c1.large", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		about: "17 cores only the flexible shape",
		cons:  "cores=17",
		expectedItypes: []string{
			"VM.Standard3.Flex",
		},
	}, {
		about: "cpu-power",
		cons:  "cpu-power=2000",
		expectedItypes: []string{
			"VM.Standard3.Flex", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		about: "mem",
		cons:  "mem=4G",
		expectedItypes: []string{
			"VM.Standard3.Flex", "m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		about: "root-disk",
		cons:  "root-disk=16G",
		expectedItypes: []string{
			"VM.Standard3.Flex", "m1.medium", "m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		about:          "arches filtered by constraint",
		cons:           "cpu-power=100 arch=arm64",
		expectedItypes: []string{"c1.large"},
		arch:           "arm64",
	},
	{
		about: "enough memory for mongodb if mem not specified",
		cons:  "cores=4",
		itypesToUse: []InstanceType{
			{Id: "5", Name: "it-5", Arch: "amd64", Mem: 1024, CpuCores: 2},
			{Id: "4", Name: "it-4", Arch: "amd64", Mem: 4096, CpuCores: 4},
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 2048, CpuCores: 4},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 256, CpuCores: 4},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512, CpuCores: 4},
		},
		expectedItypes: []string{"it-3", "it-4"},
	},
	{
		about: "small mem specified, use that even though less than needed for mongodb",
		cons:  "mem=300M",
		itypesToUse: []InstanceType{
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 2048, CpuCores: 1},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 256, CpuCores: 1},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512, CpuCores: 1},
		},
		expectedItypes: []string{"it-1", "it-3"},
	},
	{
		about: "mem specified and match found",
		cons:  "mem=4G arch=amd64",
		itypesToUse: []InstanceType{
			{Id: "4", Name: "it-4", Arch: "arm64", Mem: 8096, CpuCores: 1},
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 4096, CpuCores: 1},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 2048, CpuCores: 1},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512, CpuCores: 1},
		},
		expectedItypes: []string{"it-3"},
	},
	{
		about: "instance-type specified and match found",
		cons:  "instance-type=it-3",
		itypesToUse: []InstanceType{
			{Id: "4", Name: "it-4", Arch: "amd64", Mem: 8096},
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 4096},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 2048},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512},
		},
		expectedItypes: []string{"it-3"},
	},
	{
		about: "largest mem available matching other constraints if mem not specified",
		cons:  "cores=4",
		itypesToUse: []InstanceType{
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 1024, CpuCores: 2},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 256, CpuCores: 4},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512, CpuCores: 4},
		},
		expectedItypes: []string{"it-1"},
	},
	{
		about: "largest mem available matching other constraints if mem not specified, cost is tie breaker",
		cons:  "cores=4",
		itypesToUse: []InstanceType{
			{Id: "4", Name: "it-4", Arch: "amd64", Mem: 1024, CpuCores: 2},
			{Id: "3", Name: "it-3", Arch: "amd64", Mem: 256, CpuCores: 4},
			{Id: "2", Name: "it-2", Arch: "amd64", Mem: 512, CpuCores: 4, Cost: 50},
			{Id: "1", Name: "it-1", Arch: "amd64", Mem: 512, CpuCores: 4, Cost: 100},
		},
		expectedItypes: []string{"it-2"},
	}, {
		about:          "virt-type filtered by constraint",
		cons:           "virt-type=hvm",
		expectedItypes: []string{"cc1.4xlarge", "cc2.8xlarge"},
		itypesToUse:    nil,
	},
}

func (s *instanceTypeSuite) TestGetMatchingInstanceTypes(c *tc.C) {
	for i, t := range getInstanceTypesTest {
		c.Logf("test %d: %s", i, t.about)
		itypesToUse := t.itypesToUse
		if itypesToUse == nil {
			itypesToUse = instanceTypes
		}
		itypes, err := MatchingInstanceTypes(itypesToUse, "test", constraints.MustParse(t.cons))
		c.Assert(err, tc.ErrorIsNil)
		names := make([]string, len(itypes))
		for i, itype := range itypes {
			if t.arch != "" {
				c.Check(itype.Arch, tc.Equals, t.arch)
			} else {
				c.Check(itype.Arch, tc.Not(tc.Equals), "")
			}
			names[i] = itype.Name
		}
		c.Check(names, tc.DeepEquals, t.expectedItypes)
	}
}

func (s *instanceTypeSuite) TestGetMatchingInstanceTypesErrors(c *tc.C) {
	_, err := MatchingInstanceTypes(nil, "test", constraints.MustParse("cpu-power=9001"))
	c.Check(err, tc.ErrorMatches, `no instance types in test matching constraints "cpu-power=9001"`)

	_, err = MatchingInstanceTypes(instanceTypes, "test", constraints.MustParse("arch=arm64 mem=8G"))
	c.Check(err, tc.ErrorMatches, `no instance types in test matching constraints "arch=arm64 mem=8192M"`)

	_, err = MatchingInstanceTypes(instanceTypes, "test", constraints.MustParse("cores=9000"))
	c.Check(err, tc.ErrorMatches, `no instance types in test matching constraints "cores=9000"`)

	_, err = MatchingInstanceTypes(instanceTypes, "test", constraints.MustParse("mem=900000M"))
	c.Check(err, tc.ErrorMatches, `no instance types in test matching constraints "mem=900000M"`)

	_, err = MatchingInstanceTypes(instanceTypes, "test", constraints.MustParse("instance-type=dep.medium mem=8G"))
	c.Check(err, tc.ErrorMatches, `no instance types in test matching constraints "instance-type=dep.medium mem=8192M"`)
}

var instanceTypeMatchTests = []struct {
	cons  string
	itype string
	arch  string
}{
	{"", "m1.small", "amd64"},
	{"", "m1.large", "amd64"},
	{"cpu-power=100", "m1.small", "amd64"},
	{"arch=amd64", "m1.small", "amd64"},
	{"cores=3", "m1.xlarge", "amd64"},
	{"cpu-power=", "t1.micro", "amd64"},
	{"cpu-power=500", "c1.medium", "amd64"},
	{"cpu-power=2000", "c1.xlarge", "amd64"},
	{"cpu-power=2001", "cc1.4xlarge", "amd64"},
	{"mem=2G", "m1.medium", "amd64"},

	{"arch=arm64", "m1.small", ""},
	{"cpu-power=100", "t1.micro", ""},
	{"cpu-power=9001", "cc2.8xlarge", ""},
	{"mem=1G", "t1.micro", ""},
	{"arch=arm64", "c1.xlarge", ""},

	// Match on flexible shape against their maximum cpu-cores
	{"cores=1", "VM.Standard3.Flex", "amd64"},
	{"cores=16", "VM.Standard3.Flex", "amd64"},
	{"cores=31", "VM.Standard3.Flex", "amd64"},
	// Match on flexible shape against their maximum memory
	{"mem=1G", "VM.Standard3.Flex", "amd64"},
	{"mem=16G", "VM.Standard3.Flex", "amd64"},
	{"mem=511G", "VM.Standard3.Flex", "amd64"},
}

func (s *instanceTypeSuite) TestMatch(c *tc.C) {
	for i, t := range instanceTypeMatchTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.cons)
		var itype InstanceType
		for _, itype = range instanceTypes {
			if itype.Name == t.itype {
				break
			}
		}
		c.Assert(itype.Name, tc.Not(tc.Equals), "")
		itype, match := itype.match(cons)
		if t.arch != "" {
			c.Check(match, tc.IsTrue)
			c.Check(itype, tc.DeepEquals, itype)
		} else {
			c.Check(match, tc.IsFalse)
			c.Check(itype, tc.DeepEquals, InstanceType{})
		}
	}
}

var byCostTests = []struct {
	about          string
	itypesToUse    []InstanceType
	expectedItypes []string
}{
	{
		about: "default to lowest cost",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048, Cost: 241},
		},
		expectedItypes: []string{
			"it-2", "it-1",
		},
	}, {
		about: "when no cost associated, pick lowest ram",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cost is the same, pick lowest ram",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048, Cost: 240},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cost and ram is the same, pick lowest cpu power",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, CpuPower: CpuPower(200)},
			{Id: "1", Name: "it-1", CpuCores: 1, CpuPower: CpuPower(100)},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cpu power is the same, pick the lowest cores",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, CpuPower: CpuPower(200)},
			{Id: "1", Name: "it-1", CpuCores: 1, CpuPower: CpuPower(200)},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cpu power is missing in side a, pick the lowest cores",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, CpuPower: CpuPower(200)},
			{Id: "1", Name: "it-1", CpuCores: 1},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cpu power is missing in side b, pick the lowest cores",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2},
			{Id: "1", Name: "it-1", CpuCores: 1, CpuPower: CpuPower(200)},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		about: "when cpu cores is the same, pick the lowest root disk size",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 1, RootDisk: 8192},
			{Id: "1", Name: "it-1", CpuCores: 1, RootDisk: 4096},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	},
}

func (s *instanceTypeSuite) TestSortByCost(c *tc.C) {
	for i, t := range byCostTests {
		c.Logf("test %d: %s", i, t.about)
		sort.Sort(byCost(t.itypesToUse))
		names := make([]string, len(t.itypesToUse))
		for i, itype := range t.itypesToUse {
			names[i] = itype.Name
		}
		c.Check(names, tc.DeepEquals, t.expectedItypes)
	}
}

var byNameTests = []struct {
	about          string
	itypesToUse    []InstanceType
	expectedItypes []string
}{
	{
		about: "when different Name, pick lowest Name after delimiter",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "a1.xLarge", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "1", Name: "c2.12xLarge", CpuCores: 1, Mem: 2048, Cost: 241},
		},
		expectedItypes: []string{
			"a1.xLarge", "c2.12xLarge",
		},
	}, {
		about: "when differentsame Name before delimiter, pick lowest cost",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "a1.xLarge", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "3", Name: "a1.6xLarge", CpuCores: 2, Mem: 4096, Cost: 440},
			{Id: "1", Name: "a1.12xLarge", CpuCores: 1, Mem: 2048, Cost: 800},
		},
		expectedItypes: []string{
			"a1.xLarge", "a1.6xLarge", "a1.12xLarge",
		},
	},
	{
		about: "when name mixture, same and different, before delimiter, pick lowest cost",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "a1.xLarge", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "3", Name: "a1.6xLarge", CpuCores: 2, Mem: 4096, Cost: 440},
			{Id: "4", Name: "b1.4xLarge", CpuCores: 2, Mem: 4096, Cost: 500},
			{Id: "5", Name: "b1.2xLarge", CpuCores: 2, Mem: 4096, Cost: 400},
			{Id: "1", Name: "a1.12xLarge", CpuCores: 1, Mem: 2048, Cost: 800},
		},
		expectedItypes: []string{
			"a1.xLarge", "a1.6xLarge", "a1.12xLarge", "b1.2xLarge", "b1.4xLarge",
		},
	},
	{
		about: "when different no delimiter, base to lexical sort",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "a1xLarge", CpuCores: 2, Mem: 4096, Cost: 300},
			{Id: "1", Name: "c212xLarge", CpuCores: 1, Mem: 2048, Cost: 241},
		},
		expectedItypes: []string{
			"a1xLarge", "c212xLarge",
		},
	}, {
		about: "when different not defined delimiter, base to lexical sort",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "a1+12xlarge", CpuCores: 2, Mem: 4096, Cost: 890},
			{Id: "1", Name: "c2+12xLarge", CpuCores: 1, Mem: 2048, Cost: 800},
		},
		expectedItypes: []string{
			"a1+12xlarge", "c2+12xLarge",
		},
	},
}

func (s *instanceTypeSuite) TestSortByName(c *tc.C) {
	for i, t := range byNameTests {
		c.Logf("test %d: %s", i, t.about)
		sort.Sort(ByName(t.itypesToUse))
		names := make([]string, len(t.itypesToUse))
		for i, itype := range t.itypesToUse {
			names[i] = itype.Name
		}
		c.Check(names, tc.DeepEquals, t.expectedItypes)
	}
}

func makeUint64Pointer(val uint64) *uint64 {
	return &val
}
