package instances

import (
	"bufio"
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	coretesting "launchpad.net/juju-core/testing"
	"strings"
	"testing"
)

type imageSuite struct {
	coretesting.LoggingSuite
}

func Test(t *testing.T) {
	TestingT(t)
}

var _ = Suite(&imageSuite{})

func (s *imageSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *imageSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownTest(c)
}

var imagesData = imagesFields(
	"instance-store amd64 us-east-1 ami-00000011 paravirtual",
	"ebs amd64 eu-west-1 ami-00000016 paravirtual",
	"ebs i386 ap-northeast-1 ami-00000023 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000026 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000087 hvm",
	"ebs amd64 test ami-00000033 paravirtual",
	"ebs i386 test ami-00000034 paravirtual",
	"ebs amd64 test ami-00000035 hvm",
	"ebs i386 i386-only ami-00000036 paravirtual",
)

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

type instanceSpecTestParams struct {
	desc                string
	region              string
	arches              []string
	constraints         string
	instanceTypes       []InstanceType
	defaultImageId      string
	defaultInstanceType string
	imageId             string
	instanceTypeId      string
	instanceTypeName    string
	err                 string
}

func (p *instanceSpecTestParams) init() {
	if p.arches == nil {
		p.arches = Both
	}
	if p.instanceTypes == nil {
		p.instanceTypes = []InstanceType{{Id: "1", Name: "it-1", Arches: Both}}
		p.instanceTypeId = "1"
		p.instanceTypeName = "it-1"
	}
}

var findInstanceSpecTests = []instanceSpecTestParams{
	{
		desc:           "image exists in metadata",
		region:         "test",
		defaultImageId: "1234",
		imageId:        "ami-00000033",
	},
	{
		desc:           "no image exists in metadata, use supplied default",
		region:         "invalid-region",
		defaultImageId: "1234",
		imageId:        "1234",
	},
	{
		desc:    "no image exists in metadata, no default supplied",
		region:  "invalid-region",
		imageId: "1234",
		err:     `no "raring" images in invalid-region with arches \[amd64 i386\], and no default specified`,
	},
	{
		desc:          "no valid instance types",
		region:        "test",
		instanceTypes: []InstanceType{},
		err:           `no instance types in test matching constraints "cpu-power=100", and no default specified`,
	},
	{
		desc:          "no compatible instance types",
		region:        "i386-only",
		instanceTypes: []InstanceType{{Id: "1", Name: "it-1", Arches: Amd64, Mem: 2048}},
		err:           `no "raring" images in i386-only matching instance types \[it-1\]`,
	},
	{
		desc:        "fallback instance type, enough memory for mongodb",
		region:      "test",
		constraints: "mem=8G",
		instanceTypes: []InstanceType{
			{Id: "3", Name: "it-3", Arches: Amd64, Mem: 4096},
			{Id: "2", Name: "it-2", Arches: Amd64, Mem: 2048},
			{Id: "1", Name: "it-1", Arches: Amd64, Mem: 512},
		},
		imageId:          "ami-00000033",
		instanceTypeId:   "2",
		instanceTypeName: "it-2",
	},
	{
		desc:        "fallback instance type, not enough memory for mongodb",
		region:      "test",
		constraints: "mem=4G",
		instanceTypes: []InstanceType{
			{Id: "2", Name: "it-2", Arches: Amd64, Mem: 256},
			{Id: "1", Name: "it-1", Arches: Amd64, Mem: 512},
		},
		imageId:          "ami-00000033",
		instanceTypeId:   "1",
		instanceTypeName: "it-1",
	},
}

func (s *imageSuite) TestFindInstanceSpec(c *C) {
	for _, t := range findInstanceSpecTests {
		c.Logf("test: %v", t.desc)
		t.init()
		r := bufio.NewReader(bytes.NewBufferString(imagesData))
		spec, err := FindInstanceSpec(r, &InstanceConstraint{
			Series:         "raring",
			Region:         t.region,
			Arches:         t.arches,
			Constraints:    constraints.MustParse(t.constraints),
			DefaultImageId: t.defaultImageId,
		}, t.instanceTypes, nil)
		if t.err != "" {
			c.Check(err, ErrorMatches, t.err)
			continue
		}
		if !c.Check(err, IsNil) {
			continue
		}
		c.Check(spec.Image.Id, Equals, t.imageId)
		c.Check(spec.InstanceTypeId, Equals, t.instanceTypeId)
		c.Check(spec.InstanceTypeName, Equals, t.instanceTypeName)
	}
}

var getImagesTests = []struct {
	region string
	series string
	arches []string
	images []Image
	err    string
}{
	{
		region: "us-east-1",
		series: "precise",
		arches: Both,
		err:    `no "precise" images in us-east-1 with arches \[amd64 i386\]`,
	}, {
		region: "eu-west-1",
		series: "precise",
		arches: []string{"i386"},
		err:    `no "precise" images in eu-west-1 with arches \[i386\]`,
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: Both,
		images: []Image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
			{"ami-00000023", "i386", false},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"amd64"},
		images: []Image{
			{"ami-00000026", "amd64", false},
			{"ami-00000087", "amd64", true},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"i386"},
		images: []Image{
			{"ami-00000023", "i386", false},
		},
	},
}

func (s *imageSuite) TestGetImages(c *C) {
	var ebs = "ebs"
	var cluster = "hvm"
	for i, t := range getImagesTests {
		c.Logf("test %d", i)
		r := bufio.NewReader(bytes.NewBufferString(imagesData))
		images, err := getImages(r, &InstanceConstraint{
			Region:  t.region,
			Series:  t.series,
			Arches:  t.arches,
			Storage: &ebs,
			Cluster: &cluster,
		})
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
	image Image
	itype InstanceType
	match bool
}{
	{
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"i386", "amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", Clustered: true},
		itype: InstanceType{Arches: []string{"amd64"}, Clustered: true},
		match: true,
	}, {
		image: Image{Arch: "i386"},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64", Clustered: true},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arches: []string{"amd64"}, Clustered: true},
	},
}

func (s *imageSuite) TestImageMatch(c *C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), Equals, t.match)
	}
}
