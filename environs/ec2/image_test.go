package ec2

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/instances"
	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/testing"
	"strings"
)

type imageSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(imagesData)
}

func (s *imageSuite) TearDownSuite(c *C) {
	UseTestImageData(nil)
	s.LoggingSuite.TearDownTest(c)
}

func imagesFields(srcs ...string) string {
	strs := make([]string, len(srcs))
	for i, src := range srcs {
		parts := strings.Split(src, " ")
		if len(parts) != 5 {
			panic("bad clouddata field input")
		}
		args := make([]interface{}, len(parts))
		for i, part := range parts {
			args[i] = part
		}
		// Ignored fields are left empty for clarity's sake, and two additional
		// tabs are tacked on to the end to verify extra columns are ignored.
		strs[i] = fmt.Sprintf("\t\t\t\t%s\t%s\t%s\t%s\t\t\t%s\t\t\n", args...)
	}
	return strings.Join(strs, "")
}

var imagesData = []jujutest.FileContent{
	{"/query/precise/server/released.current.txt", imagesFields(
		"instance-store amd64 us-east-1 ami-00000011 paravirtual",
		"ebs amd64 eu-west-1 ami-00000016 paravirtual",
		"ebs i386 ap-northeast-1 ami-00000023 paravirtual",
		"ebs amd64 ap-northeast-1 ami-00000026 paravirtual",
		"ebs amd64 ap-northeast-1 ami-00000087 hvm",
		"ebs amd64 test ami-00000033 paravirtual",
		"ebs i386 test ami-00000034 paravirtual",
		"ebs amd64 test ami-00000035 hvm",
	)},
	{"/query/quantal/server/released.current.txt", imagesFields(
		"instance-store amd64 us-east-1 ami-00000011 paravirtual",
		"ebs amd64 eu-west-1 ami-01000016 paravirtual",
		"ebs i386 ap-northeast-1 ami-01000023 paravirtual",
		"ebs amd64 ap-northeast-1 ami-01000026 paravirtual",
		"ebs amd64 ap-northeast-1 ami-01000087 hvm",
		"ebs i386 test ami-01000034 paravirtual",
		"ebs amd64 test ami-01000035 hvm",
	)},
	{"/query/raring/server/released.current.txt", imagesFields(
		"ebs i386 test ami-02000034 paravirtual",
	)},
}

var instanceTypeCosts = instances.InstanceTypeCost{
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

type specSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&specSuite{})

func (s *specSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(imagesData)
	UseTestInstanceTypeData(instanceTypeCosts)
}

func (s *specSuite) TearDownSuite(c *C) {
	UseTestInstanceTypeData(nil)
	UseTestImageData(nil)
	s.LoggingSuite.TearDownSuite(c)
}

var findInstanceSpecTests = []struct {
	series string
	arches []string
	cons   string
	itype  string
	image  string
}{
	{
		series: "precise",
		arches: instances.Both,
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "quantal",
		arches: instances.Both,
		itype:  "m1.small",
		image:  "ami-01000034",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-cores=4",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-cores=2 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "mem=10G",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "mem=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-power=",
		itype:  "t1.micro",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-power=800",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-power=500 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: []string{"i386"},
		cons:   "cpu-power=400",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "quantal",
		arches: instances.Both,
		cons:   "arch=amd64",
		itype:  "cc1.4xlarge",
		image:  "ami-01000035",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *C) {
	for i, t := range findInstanceSpecTests {
		c.Logf("test %d", i)
		spec, err := findInstanceSpec(&instances.InstanceConstraint{
			Region:      "test",
			Series:      t.series,
			Arches:      t.arches,
			Constraints: constraints.MustParse(t.cons),
			Cluster:     &cluster,
			Storage:     &ebsStorage,
		})
		c.Assert(err, IsNil)
		c.Check(spec.InstanceTypeName, Equals, t.itype)
		c.Check(spec.Image.Id, Equals, t.image)
	}
}

var findInstanceSpecErrorTests = []struct {
	series string
	arches []string
	cons   string
	err    string
}{
	{
		series: "bad",
		arches: instances.Both,
		err:    `no "bad" images in test with arches \[amd64 i386\], and no default specified`,
	}, {
		series: "precise",
		arches: []string{"arm"},
		err:    `no "precise" images in test with arches \[arm\], and no default specified`,
	}, {
		series: "precise",
		arches: instances.Both,
		cons:   "cpu-power=9001",
		err:    `no instance types in test matching constraints "cpu-power=9001", and no default specified`,
	}, {
		series: "raring",
		arches: instances.Both,
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[m1.large m1.xlarge c1.xlarge cc1.4xlarge cc2.8xlarge\]`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		_, err := findInstanceSpec(&instances.InstanceConstraint{
			Region:      "test",
			Series:      t.series,
			Arches:      t.arches,
			Constraints: constraints.MustParse(t.cons),
		})
		c.Check(err, ErrorMatches, t.err)
	}
}
