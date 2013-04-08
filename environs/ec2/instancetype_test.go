package ec2

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
	UseTestInstanceTypeData(instanceTypeData)
}

func (s *instanceTypeSuite) TearDownSuite(c *C) {
	UseTestInstanceTypeData(nil)
	s.LoggingSuite.TearDownTest(c)
}

var instanceTypeData = map[string]uint64{
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

var getInstanceTypesTest = []struct {
	info   string
	cons   string
	itypes []string
	arches []string
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
		cons:   "arch=i386",
		itypes: []string{"m1.small", "m1.medium", "c1.medium"},
		arches: []string{"i386"},
	}, {
		info: "t1.micro filtered out when no cpu-power set",
		itypes: []string{
			"m1.small", "m1.medium", "c1.medium", "m1.large", "m1.xlarge",
			"c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info: "t1.micro included when small cpu-power set",
		cons: "cpu-power=1",
		itypes: []string{
			"t1.micro", "m1.small", "m1.medium", "c1.medium", "m1.large",
			"m1.xlarge", "c1.xlarge", "cc1.4xlarge", "cc2.8xlarge",
		},
	}, {
		info:   "t1.micro included when small cpu-power set 2",
		cons:   "cpu-power=1 arch=i386",
		itypes: []string{"t1.micro", "m1.small", "m1.medium", "c1.medium"},
		arches: []string{"i386"},
	},
}

func (s *instanceTypeSuite) TestGetInstanceTypes(c *C) {
	for i, t := range getInstanceTypesTest {
		c.Logf("test %d: %s", i, t.info)
		itypes, err := getInstanceTypes("test", constraints.MustParse(t.cons))
		c.Assert(err, IsNil)
		names := make([]string, len(itypes))
		for i, itype := range itypes {
			if len(t.arches) > 0 {
				c.Check(itype.arches, DeepEquals, filterArches(itype.arches, t.arches))
			} else {
				c.Check(len(itype.arches) > 0, Equals, true)
			}
			names[i] = itype.name
		}
		c.Check(names, DeepEquals, t.itypes)
	}
}

func (s *instanceTypeSuite) TestGetInstanceTypesErrors(c *C) {
	_, err := getInstanceTypes("unknown-region", constraints.Value{})
	c.Check(err, ErrorMatches, `no instance types found in unknown-region`)

	cons := constraints.MustParse("cpu-power=9001")
	_, err = getInstanceTypes("test", cons)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "cpu-power=9001"`)

	cons = constraints.MustParse("arch=i386 mem=8G")
	_, err = getInstanceTypes("test", cons)
	c.Check(err, ErrorMatches, `no instance types in test matching constraints "arch=i386 cpu-power=100 mem=8192M"`)
}

var instanceTypeMatchTests = []struct {
	cons   string
	itype  string
	arches []string
}{
	{"", "m1.small", both},
	{"", "m1.large", amd64},
	{"cpu-power=100", "m1.small", both},
	{"arch=amd64", "m1.small", amd64},
	{"cpu-cores=3", "m1.xlarge", amd64},
	{"cpu-power=", "t1.micro", both},
	{"cpu-power=500", "c1.medium", both},
	{"cpu-power=2000", "c1.xlarge", amd64},
	{"cpu-power=2001", "cc1.4xlarge", amd64},
	{"mem=2G", "m1.medium", both},

	{"arch=arm", "m1.small", nil},
	{"cpu-power=100", "t1.micro", nil},
	{"cpu-power=9001", "cc2.8xlarge", nil},
	{"mem=1G", "t1.micro", nil},
	{"arch=i386", "c1.xlarge", nil},
}

func (s *instanceTypeSuite) TestMatch(c *C) {
	for i, t := range instanceTypeMatchTests {
		c.Logf("test %d", i)
		cons := constraints.MustParse(t.cons)
		var itype instanceType
		for _, itype = range allInstanceTypes {
			if itype.name == t.itype {
				break
			}
		}
		c.Assert(itype.name, Not(Equals), "")
		itype, match := itype.match(cons)
		if len(t.arches) > 0 {
			c.Check(match, Equals, true)
			expect := itype
			expect.arches = t.arches
			c.Check(itype, DeepEquals, expect)
		} else {
			c.Check(match, Equals, false)
			c.Check(itype, DeepEquals, instanceType{})
		}
	}
}
