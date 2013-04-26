package instances

import (
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

var instanceTypeCosts = InstanceTypeCost{
	"m1.small":    60,
	"m1.medium":   120,
	"m1.large":    240,
	"m1.xlarge":   480,
	"t1.micro":    20,
	"c1.medium":   145,
	"c1.xlarge":   580,
	"cc1.4xlarge": 1300,
	"cc2.8xlarge": 2400,
}

var regionCosts = RegionCosts{
	"test": instanceTypeCosts,
}

var hvm = "hvm"

var instanceTypes = []InstanceType{
	{
		Name:     "m1.small",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(100),
		Mem:      1740,
	}, {
		Name:     "m1.medium",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(200),
		Mem:      3840,
	}, {
		Name:     "m1.large",
		Arches:   []string{"amd64"},
		CpuCores: 2,
		CpuPower: CpuPower(400),
		Mem:      7680,
	}, {
		Name:     "m1.xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 4,
		CpuPower: CpuPower(800),
		Mem:      15360,
	},
	{
		Name:     "t1.micro",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 1,
		CpuPower: CpuPower(20),
		Mem:      613,
	},
	{
		Name:     "c1.medium",
		Arches:   []string{"amd64", "arm"},
		CpuCores: 2,
		CpuPower: CpuPower(500),
		Mem:      1740,
	}, {
		Name:     "c1.xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 8,
		CpuPower: CpuPower(2000),
		Mem:      7168,
	},
	{
		Name:     "cc1.4xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 8,
		CpuPower: CpuPower(3350),
		Mem:      23552,
		VType:    &hvm,
	}, {
		Name:     "cc2.8xlarge",
		Arches:   []string{"amd64"},
		CpuCores: 16,
		CpuPower: CpuPower(8800),
		Mem:      61952,
		VType:    &hvm,
	},
}

var getInstanceTypesTest = []struct {
	info    string
	cons    string
	itypes  []string
	arches  []string
	noCosts bool
}{
	{
		info: "cpu-cores",
		cons: "cpu-cores=2",
		itypes: []string{
			"c1.medium", "m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge",
			"cc2.8xlarge",
		},
	}, {
		info:   "cpu-power",
		cons:   "cpu-power=2000",
		itypes: []string{"c1.xlarge", "cc1.4xlarge", "cc2.8xlarge"},
	}, {
		info: "mem",
		cons: "mem=4G",
		itypes: []string{
			"m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:   "arches filtered by constraint",
		cons:   "cpu-power=100 arch=arm",
		itypes: []string{"m1.small", "m1.medium", "c1.medium"},
		arches: []string{"arm"},
	}, {
		info: "no costs data available",
		cons: "cpu-cores=2",
		itypes: []string{
			"c1.medium", "c1.xlarge", "m1.large", "m1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
		noCosts: true,
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
		var costs RegionCosts
		if !t.noCosts {
			costs = regionCosts
		}
		itypes, err := getMatchingInstanceTypes(constraint("test", t.cons), instanceTypes, costs)
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
		c.Check(names, DeepEquals, t.itypes)
	}
}

func (s *instanceTypeSuite) TestGetMatchingInstanceTypesErrors(c *C) {
	_, err := getMatchingInstanceTypes(constraint("unknown-region", ""), instanceTypes, regionCosts)
	c.Check(err, ErrorMatches, `no instance types found in unknown-region`)

	_, err = getMatchingInstanceTypes(constraint("test", "cpu-power=9001"), instanceTypes, regionCosts)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "cpu-power=9001", and no default specified`)

	_, err = getMatchingInstanceTypes(constraint("test", "arch=arm mem=8G"), instanceTypes, regionCosts)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "arch=arm mem=8192M", and no default specified`)
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
