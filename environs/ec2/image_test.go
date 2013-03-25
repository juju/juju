package ec2

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
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

var getImagesTests = []struct {
	region string
	series string
	arches []string
	images []image
	err    string
}{
	{
		region: "us-east-1",
		series: "precise",
		arches: both,
		err:    `no "precise" images in us-east-1 with arches \[amd64 i386\]`,
	}, {
		region: "eu-west-1",
		series: "precise",
		arches: []string{"i386"},
		err:    `no "precise" images in eu-west-1 with arches \[i386\]`,
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: both,
		images: []image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
			{"ami-00000023", "i386", false},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"amd64"},
		images: []image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"i386"},
		images: []image{
			{"ami-00000023", "i386", false},
		},
	}, {
		region: "ap-northeast-1",
		series: "quantal",
		arches: both,
		images: []image{
			{"ami-01000026", "amd64", false},
			{"ami-01000087", "amd64", true},
			{"ami-01000023", "i386", false},
		},
	},
}

func (s *imageSuite) TestGetImages(c *C) {
	for i, t := range getImagesTests {
		c.Logf("test %d", i)
		images, err := getImages(t.region, t.series, t.arches)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		c.Check(images, DeepEquals, t.images)
	}
}

var imageMatchtests = []struct {
	image image
	itype instanceType
	match bool
}{
	{
		image: image{arch: "amd64"},
		itype: instanceType{arches: []string{"amd64"}},
		match: true,
	}, {
		image: image{arch: "amd64"},
		itype: instanceType{arches: []string{"i386", "amd64"}},
		match: true,
	}, {
		image: image{arch: "amd64", hvm: true},
		itype: instanceType{arches: []string{"amd64"}, hvm: true},
		match: true,
	}, {
		image: image{arch: "i386"},
		itype: instanceType{arches: []string{"amd64"}},
	}, {
		image: image{arch: "amd64", hvm: true},
		itype: instanceType{arches: []string{"amd64"}},
	}, {
		image: image{arch: "amd64"},
		itype: instanceType{arches: []string{"amd64"}, hvm: true},
	},
}

func (s *imageSuite) TestImageMatch(c *C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), Equals, t.match)
	}
}

type specSuite struct {
	testing.LoggingSuite
}

var _ = Suite(&specSuite{})

func (s *specSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
	UseTestImageData(imagesData)
	UseTestInstanceTypeData(instanceTypeData)
}

func (s *specSuite) TearDownSuite(c *C) {
	UseTestInstanceTypeData(nil)
	UseTestImageData(nil)
	s.LoggingSuite.TearDownTest(c)
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
		arches: both,
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "quantal",
		arches: both,
		itype:  "m1.small",
		image:  "ami-01000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=4",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-cores=2 arch=i386",
		itype:  "c1.medium",
		image:  "ami-00000034",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=10G",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "mem=",
		itype:  "m1.small",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=",
		itype:  "t1.micro",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=800",
		itype:  "m1.xlarge",
		image:  "ami-00000033",
	}, {
		series: "precise",
		arches: both,
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
		arches: both,
		cons:   "arch=amd64",
		itype:  "cc1.4xlarge",
		image:  "ami-01000035",
	},
}

func (s *specSuite) TestFindInstanceSpec(c *C) {
	for i, t := range findInstanceSpecTests {
		c.Logf("test %d", i)
		cons, err := constraints.Parse(t.cons)
		c.Assert(err, IsNil)
		spec, err := findInstanceSpec(&instanceConstraint{
			region:      "test",
			series:      t.series,
			arches:      t.arches,
			constraints: cons,
		})
		c.Assert(err, IsNil)
		c.Check(spec.instanceType, Equals, t.itype)
		c.Check(spec.image.id, Equals, t.image)
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
		arches: both,
		err:    `cannot get image data for "bad": .*`,
	}, {
		series: "precise",
		arches: []string{"arm"},
		err:    `no "precise" images in test with arches \[arm\]`,
	}, {
		series: "precise",
		arches: both,
		cons:   "cpu-power=9001",
		err:    `no instance types in test matching constraints "cpu-power=9001"`,
	}, {
		series: "raring",
		arches: both,
		cons:   "mem=4G",
		err:    `no "raring" images in test matching instance types \[m1.large m1.xlarge c1.xlarge cc1.4xlarge cc2.8xlarge\]`,
	},
}

func (s *specSuite) TestFindInstanceSpecErrors(c *C) {
	for i, t := range findInstanceSpecErrorTests {
		c.Logf("test %d", i)
		cons, err := constraints.Parse(t.cons)
		c.Assert(err, IsNil)
		_, err = findInstanceSpec(&instanceConstraint{
			region:      "test",
			series:      t.series,
			arches:      t.arches,
			constraints: cons,
		})
		c.Check(err, ErrorMatches, t.err)
	}
}
