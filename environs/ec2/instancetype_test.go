package ec2

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/testing"
)

type instanceTypeSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&instanceTypeSuite{})

func (s *instanceTypeSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestInstanceTypeData(instanceTypeData)
}

func (s *instanceTypeSuite) TearDownSuite(c *C) {
	UseTestInstanceTypeData(nil)
	s.LoggingSuite.TearDownTest(c)
}

var instanceTypeData = map[string]float64{
	"m1.small":    0.060,
	"m1.medium":   0.120,
	"m1.large":    0.240,
	"m1.xlarge":   0.480,
	"t1.micro":    0.020,
	"c1.medium":   0.145,
	"c1.xlarge":   0.580,
	"cc1.4xlarge": 1.300,
	"cc2.8xlarge": 2.400,
}

var getInstanceTypesTest = []struct {
	info   string
	cons   string
	arches []string
	itypes []string
}{
	{
		info:   "cpu-cores",
		cons:   "cpu-cores=2",
		arches: both,
		itypes: []string{
			"c1.medium", "m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge",
			"cc2.8xlarge",
		},
	}, {
		info:   "cpu-power",
		cons:   "cpu-power=2000",
		arches: both,
		itypes: []string{"c1.xlarge", "cc1.4xlarge", "cc2.8xlarge"},
	}, {
		info:   "mem",
		cons:   "mem=4G",
		arches: both,
		itypes: []string{
			"m1.large", "m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:   "arches filtered by constraint",
		cons:   "arch=i386",
		arches: both,
		itypes: []string{"m1.small", "m1.medium", "c1.medium"},
	}, {
		info:   "arches filtered by availability",
		arches: []string{"i386"},
		itypes: []string{"m1.small", "m1.medium", "c1.medium"},
	}, {
		info:   "t1.micro filtered out when no cpu-power set",
		arches: both,
		itypes: []string{
			"m1.small", "m1.medium", "c1.medium", "m1.large", "m1.xlarge",
			"c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:   "t1.micro included when small cpu-power set",
		cons:   "cpu-power=1",
		arches: both,
		itypes: []string{
			"t1.micro", "m1.small", "m1.medium", "c1.medium", "m1.large",
			"m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:   "t1.micro included when small cpu-power set 2",
		cons:   "cpu-power=1",
		arches: []string{"i386"},
		itypes: []string{"t1.micro", "m1.small", "m1.medium", "c1.medium"},
	},
}

func (s *instanceTypeSuite) TestGetInstanceTypes(c *C) {
	for i, t := range getInstanceTypesTest {
		c.Logf("test %d: %s", i, t.info)
		cons, err := state.ParseConstraints(t.cons)
		c.Assert(err, IsNil)
		itypes, err := getInstanceTypes("test", cons, t.arches)
		c.Assert(err, IsNil)
		names := make([]string, len(itypes))
		for i, itype := range itypes {
			c.Check(itype.arches, DeepEquals, filterArches(itype.arches, t.arches))
			names[i] = itype.name
		}
		c.Check(names, DeepEquals, t.itypes)
	}
}

func (s *instanceTypeSuite) TestGetInstanceTypesErrors(c *C) {
	_, err := getInstanceTypes("unknown-region", state.Constraints{}, both)
	c.Assert(err, ErrorMatches, `no instance types found in unknown-region`)

	cons, err := state.ParseConstraints("arch=i386")
	c.Assert(err, IsNil)
	_, err = getInstanceTypes("test", cons, amd64)
	c.Assert(err, ErrorMatches, `no suitable instance types found in test`)

	cons, err = state.ParseConstraints("arch=i386 mem=8G")
	c.Assert(err, IsNil)
	_, err = getInstanceTypes("test", cons, both)
	c.Assert(err, ErrorMatches, `no suitable instance types found in test`)
}

var instanceTypeMatchTests = []struct {
	cons   string
	arches []string
	itype  string
	match  []string
}{
	{"", both, "m1.small", both},
	{"", amd64, "m1.small", amd64},
	{"", both, "m1.large", amd64},
	{"cpu-power=100", both, "m1.small", both},
	{"arch=amd64", both, "m1.small", amd64},
	{"cpu-cores=3", both, "m1.xlarge", amd64},
	{"cpu-power=", both, "t1.micro", both},
	{"cpu-power=500", both, "c1.medium", both},
	{"cpu-power=2000", both, "c1.xlarge", amd64},
	{"cpu-power=2001", both, "cc1.4xlarge", amd64},
	{"mem=2G", amd64, "m1.medium", amd64},
	{"mem=2G", both, "m1.medium", both},

	{"", nil, "m1.small", nil},
	{"arch=arm", amd64, "m1.small", nil},
	{"cpu-power=100", amd64, "t1.micro", nil},
	{"cpu-power=9001", amd64, "cc2.8xlarge", nil},
	{"mem=1G", amd64, "t1.micro", nil},
	{"arch=i386", both, "c1.xlarge", nil},
}

func (s *instanceTypeSuite) TestMatch(c *C) {
	for i, t := range instanceTypeMatchTests {
		c.Logf("test %d", i)
		cons, err := state.ParseConstraints(t.cons)
		c.Assert(err, IsNil)
		var itype instanceType
		for _, itype = range allInstanceTypes {
			if itype.name == t.itype {
				break
			}
		}
		c.Assert(itype.name, Not(Equals), "")
		itype, match := itype.match(cons, t.arches)
		if len(t.match) > 0 {
			c.Check(match, Equals, true)
			expect := itype
			expect.arches = t.match
			c.Check(itype, DeepEquals, expect)
		} else {
			c.Check(match, Equals, false)
			c.Check(itype, DeepEquals, instanceType{})
		}
	}
}
