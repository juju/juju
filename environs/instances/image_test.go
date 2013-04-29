package instances

import (
	"bufio"
	"bytes"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	coretesting "launchpad.net/juju-core/testing"
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

var imagesData = envtesting.ImagesFields(
	"instance-store amd64 us-east-1 ami-00000011 paravirtual",
	"ebs amd64 eu-west-1 ami-00000016 paravirtual",
	"ebs arm ap-northeast-1 ami-00000023 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000026 paravirtual",
	"ebs amd64 ap-northeast-1 ami-00000087 hvm",
	"ebs amd64 test ami-00000033 paravirtual",
	"ebs arm test ami-00000034 paravirtual",
	"ebs amd64 test ami-00000035 hvm",
	"ebs arm arm-only ami-00000036 paravirtual",
)

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
		p.arches = []string{"amd64", "arm"}
	}
	if p.instanceTypes == nil {
		p.instanceTypes = []InstanceType{{Id: "1", Name: "it-1", Arches: []string{"amd64", "arm"}}}
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
		err:     `no "raring" images in invalid-region with arches \[amd64 arm\], and no default specified`,
	},
	{
		desc:          "no valid instance types",
		region:        "test",
		instanceTypes: []InstanceType{},
		err:           `no instance types in test matching constraints "", and no default specified`,
	},
	{
		desc:          "no compatible instance types",
		region:        "arm-only",
		instanceTypes: []InstanceType{{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 2048}},
		err:           `no "raring" images in arm-only matching instance types \[it-1\]`,
	},
	{
		desc:        "fallback instance type, enough memory for mongodb",
		region:      "test",
		constraints: "mem=8G",
		instanceTypes: []InstanceType{
			{Id: "3", Name: "it-3", Arches: []string{"amd64"}, Mem: 4096},
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, Mem: 2048},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 512},
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
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, Mem: 256},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 512},
		},
		imageId:          "ami-00000033",
		instanceTypeId:   "1",
		instanceTypeName: "it-1",
	},
}

//func (s *imageSuite) TestFindInstanceSpec(c *C) {
//	for _, t := range findInstanceSpecTests {
//		c.Logf("test: %v", t.desc)
//		t.init()
//		r := bufio.NewReader(bytes.NewBufferString(imagesData))
//		spec, err := FindInstanceSpec(r, &InstanceConstraint{
//			Series:         "raring",
//			Region:         t.region,
//			Arches:         t.arches,
//			Constraints:    constraints.MustParse(t.constraints),
//			DefaultImageId: t.defaultImageId,
//		}, t.instanceTypes)
//		if t.err != "" {
//			c.Check(err, ErrorMatches, t.err)
//			continue
//		}
//		if !c.Check(err, IsNil) {
//			continue
//		}
//		c.Check(spec.Image.Id, Equals, t.imageId)
//		c.Check(spec.InstanceTypeId, Equals, t.instanceTypeId)
//		c.Check(spec.InstanceTypeName, Equals, t.instanceTypeName)
//	}
//}

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
		arches: []string{"amd64", "arm"},
		err:    `no "precise" images in us-east-1 with arches \[amd64 arm\]`,
	}, {
		region: "eu-west-1",
		series: "precise",
		arches: []string{"arm"},
		err:    `no "precise" images in eu-west-1 with arches \[arm\]`,
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"amd64", "arm"},
		images: []Image{
			{"ami-00000026", "amd64", "paravirtual"},
			{"ami-00000087", "amd64", "hvm"},
			{"ami-00000023", "arm", "paravirtual"},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"amd64"},
		images: []Image{
			{"ami-00000026", "amd64", "paravirtual"},
			{"ami-00000087", "amd64", "hvm"},
		},
	}, {
		region: "ap-northeast-1",
		series: "precise",
		arches: []string{"arm"},
		images: []Image{
			{"ami-00000023", "arm", "paravirtual"},
		},
	},
}

//func (s *imageSuite) TestGetImages(c *C) {
//	var ebs = "ebs"
//	var cluster = "hvm"
//	for i, t := range getImagesTests {
//		c.Logf("test %d", i)
//		r := bufio.NewReader(bytes.NewBufferString(imagesData))
//		images, err := getImages(r, &InstanceConstraint{
//			Region:  t.region,
//			Series:  t.series,
//			Arches:  t.arches,
//			Storage: &ebs,
//			Cluster: &cluster,
//		})
//		if t.err != "" {
//			c.Check(err, ErrorMatches, t.err)
//			continue
//		}
//		if !c.Check(err, IsNil) {
//			continue
//		}
//		c.Check(images, DeepEquals, t.images)
//	}
//}

func (s *imageSuite) TestGetImages(c *C) {
	var ebs = "ebs"
	for i, t := range getImagesTests[:1] {
		c.Logf("test %d", i)
		//		r := bufio.NewReader(bytes.NewBufferString(imagesData))
		aa, err := GetImages("aws", nil, nil, &InstanceConstraint{
			Region:  t.region,
			Series:  t.series,
			Arches:  t.arches,
			Storage: &ebs,
		})
		fmt.Println(aa)
		fmt.Println(err)
		fmt.Println("-------------------------------------")
		//		if t.err != "" {
		//			c.Check(err, ErrorMatches, t.err)
		//			continue
		//		}
		//		if !c.Check(err, IsNil) {
		//			continue
		//		}
		//		c.Check(images, DeepEquals, t.images)
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
		itype: InstanceType{Arches: []string{"amd64", "arm"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", VType: hvm},
		itype: InstanceType{Arches: []string{"amd64"}, VType: &hvm},
		match: true,
	}, {
		image: Image{Arch: "arm"},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64", VType: hvm},
		itype: InstanceType{Arches: []string{"amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", VType: "paravirtual"},
		itype: InstanceType{Arches: []string{"amd64"}, VType: &hvm},
	},
}

func (s *imageSuite) TestImageMatch(c *C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), Equals, t.match)
	}
}
