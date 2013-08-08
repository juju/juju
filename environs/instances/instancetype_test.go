// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"sort"

	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/testing"
)

type instanceTypeSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&instanceTypeSuite{})

func (s *instanceTypeSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *instanceTypeSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownTest(c)
}

var hvm = "hvm"

var instanceTypes = []InstanceType{
	{
		Name:     "m1.small",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(100),
		Mem:      1740,
		Cost:     60,
	}, {
		Name:     "m1.medium",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(200),
		Mem:      3840,
		Cost:     120,
	}, {
		Name:     "m1.large",
		Arches:   []string{"amd64"},
		CpuCores: 2,
		CpuPower: CpuPower(400),
		Mem:      7680,
		Cost:     240,
	}, {
		Name:     "m1.xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 4,
		CpuPower: CpuPower(800),
		Mem:      15360,
		Cost:     480,
	},
	{
		Name:     "t1.micro",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(20),
		Mem:      613,
		Cost:     20,
	},
	{
		Name:     "c1.medium",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 2,
		CpuPower: CpuPower(500),
		Mem:      1740,
		Cost:     145,
	}, {
		Name:     "c1.xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 8,
		CpuPower: CpuPower(2000),
		Mem:      7168,
		Cost:     580,
	},
	{
		Name:     "cc1.4xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 8,
		CpuPower: CpuPower(3350),
		Mem:      23552,
		Cost:     1300,
		VType:    &hvm,
	}, {
		Name:     "cc2.8xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 16,
		CpuPower: CpuPower(8800),
		Mem:      61952,
		Cost:     2400,
		VType:    &hvm,
	},
}

var getInstanceTypesTest = []struct {
	info           string
	cons           string
	itypesToUse    []InstanceType
	expectedItypes []string
	arches         []string
}{
	{
		info: "cpu-cores",
		cons: "cpu-cores=2",
		expectedItypes: []string{
			"c1.medium", "m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge",
			"cc2.8xlarge",
		},
	}, {
		info:           "cpu-power",
		cons:           "cpu-power=2000",
		expectedItypes: []string{"c1.xlarge", "cc1.4xlarge", "cc2.8xlarge"},
	}, {
		info: "mem",
		cons: "mem=4G",
		expectedItypes: []string{
			"m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:           "arches filtered by constraint",
		cons:           "cpu-power=100 arch=arm",
		expectedItypes: []string{"m1.small", "m1.medium", "c1.medium"},
		arches:         []string{"arm"},
	},
	{
		info: "fallback instance type, enough memory for mongodb",
		cons: "mem=8G",
		itypesToUse: []InstanceType{
			{Id: "3", Name: "it-3", Arches: []string{"amd64"}, Mem: 4096},
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, Mem: 2048},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 512},
		},
		expectedItypes: []string{"it-2"},
	},
	{
		info: "fallback instance type, not enough memory for mongodb",
		cons: "mem=4G",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, Mem: 256},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 512},
		},
		expectedItypes: []string{"it-1"},
	},
}

func constraint(region, cons string) *InstanceConstraint {
	return &InstanceConstraint{
		Region:      region,
		Constraints: constraints.MustParse(cons),
	}
}

func (s *instanceTypeSuite) TestGetMatchingInstanceTypes(c *C) {
	for i, t := range getInstanceTypesTest {
		c.Logf("test %d: %s", i, t.info)
		itypesToUse := t.itypesToUse
		if itypesToUse == nil {
			itypesToUse = instanceTypes
		}
		itypes, err := getMatchingInstanceTypes(constraint("test", t.cons), itypesToUse)
		c.Assert(err, IsNil)
		names := make([]string, len(itypes))
		for i, itype := range itypes {
			if len(t.arches) > 0 {
				c.Check(itype.Arches, DeepEquals, filterArches(itype.Arches, t.arches))
			} else {
				c.Check(len(itype.Arches) > 0, Equals, true)
			}
			names[i] = itype.Name
		}
		c.Check(names, DeepEquals, t.expectedItypes)
	}
}

func (s *instanceTypeSuite) TestGetMatchingInstanceTypesErrors(c *C) {
	_, err := getMatchingInstanceTypes(constraint("test", "cpu-power=9001"), nil)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "cpu-power=9001"`)

	_, err = getMatchingInstanceTypes(constraint("test", "arch=i386 mem=8G"), instanceTypes)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "arch=i386 mem=8192M"`)
}

var instanceTypeMatchTests = []struct {
	cons   string
	itype  string
	arches []string
}{
	{"", "m1.small", []string{"amd64", "arm"}},
	{"", "m1.large", []string{"amd64"}},
	{"cpu-power=100", "m1.small", []string{"amd64", "arm"}},
	{"arch=amd64", "m1.small", []string{"amd64"}},
	{"cpu-cores=3", "m1.xlarge", []string{"amd64"}},
	{"cpu-power=", "t1.micro", []string{"amd64", "arm"}},
	{"cpu-power=500", "c1.medium", []string{"amd64", "arm"}},
	{"cpu-power=2000", "c1.xlarge", []string{"amd64"}},
	{"cpu-power=2001", "cc1.4xlarge", []string{"amd64"}},
	{"mem=2G", "m1.medium", []string{"amd64", "arm"}},

	{"arch=i386", "m1.small", nil},
	{"cpu-power=100", "t1.micro", nil},
	{"cpu-power=9001", "cc2.8xlarge", nil},
	{"mem=1G", "t1.micro", nil},
	{"arch=arm", "c1.xlarge", nil},
}

func (s *instanceTypeSuite) TestMatch(c *C) {
	for i, t := range instanceTypeMatchTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.cons)
		var itype InstanceType
		for _, itype = range instanceTypes {
			if itype.Name == t.itype {
				break
			}
		}
		c.Assert(itype.Name, Not(Equals), "")
		itype, match := itype.match(cons)
		if len(t.arches) > 0 {
			c.Check(match, Equals, true)
			expect := itype
			expect.Arches = t.arches
			c.Check(itype, DeepEquals, expect)
		} else {
			c.Check(match, Equals, false)
			c.Check(itype, DeepEquals, InstanceType{})
		}
	}
}

var byCostTest = []struct {
	info           string
	itypesToUse    []InstanceType
	expectedItypes []string
}{
	{
		info: "default to lowest cost",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048, Cost: 241},
		},
		expectedItypes: []string{
			"it-2", "it-1",
		},
	}, {
		info: "when no cost associated, pick lowest ram",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		info: "when cost is the same, pick lowest ram",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, Mem: 4096, Cost: 240},
			{Id: "1", Name: "it-1", CpuCores: 1, Mem: 2048, Cost: 240},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		info: "when cost and ram is the same, pick lowest cpu power",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, CpuPower: CpuPower(200)},
			{Id: "1", Name: "it-1", CpuCores: 1, CpuPower: CpuPower(100)},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	}, {
		info: "when cpu power is the same, pick the lowest cores",
		itypesToUse: []InstanceType{
			{Id: "2", Name: "it-2", CpuCores: 2, CpuPower: CpuPower(200)},
			{Id: "1", Name: "it-1", CpuCores: 1, CpuPower: CpuPower(200)},
		},
		expectedItypes: []string{
			"it-1", "it-2",
		},
	},
}

func (s *instanceTypeSuite) TestSortByCost(c *C) {
	for i, t := range byCostTest {
		c.Logf("test %d: %s", i, t.info)
		sort.Sort(byCost(t.itypesToUse))
		names := make([]string, len(t.itypesToUse))
		for i, itype := range t.itypesToUse {
			names[i] = itype.Name
		}
		c.Check(names, DeepEquals, t.expectedItypes)
	}
}
