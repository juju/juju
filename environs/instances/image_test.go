// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
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

var jsonImagesContent = `
{
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
   "com.ubuntu.cloud:server:12.04:amd64": {
     "release": "precise",
     "version": "12.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "usee1pi": {
             "root_store": "instance",
             "virt": "pv",
             "region": "us-east-1",
             "id": "ami-00000011"
           },
           "usww1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "us-west-1",
             "id": "ami-00000016"
           },
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000026"
           },
           "apne1he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "ap-northeast-1",
             "id": "ami-00000087"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000033"
           },
           "test1he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "test",
             "id": "ami-00000035"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
         "label": "release"
       },
       "20121118": {
         "items": {
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000008"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121118",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:arm": {
     "release": "precise",
     "version": "12.04",
     "arch": "arm",
     "versions": {
       "20121218": {
         "items": {
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-00000023"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-00000034"
           },
           "armo1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "arm-only",
             "id": "ami-00000036"
           }
         },
         "pubname": "ubuntu-precise-12.04-arm-server-20121218",
         "label": "release"
       }
     }
   }
 },
 "format": "products:1.0"
}
`

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

var pv = "pv"
var findInstanceSpecTests = []instanceSpecTestParams{
	{
		desc:           "image exists in metadata",
		region:         "test",
		defaultImageId: "1234",
		imageId:        "ami-00000033",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VType: &pv, Mem: 512},
		},
		instanceTypeId:   "1",
		instanceTypeName: "it-1",
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
		err:     `no "precise" images in invalid-region with arches \[amd64 arm\], and no default specified`,
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
		err:           `no "precise" images in arm-only matching instance types \[it-1\]`,
	},
	{
		desc:        "fallback instance type, enough memory for mongodb",
		region:      "test",
		constraints: "mem=8G",
		instanceTypes: []InstanceType{
			{Id: "3", Name: "it-3", Arches: []string{"amd64"}, VType: &pv, Mem: 4096},
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, VType: &pv, Mem: 2048},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VType: &pv, Mem: 512},
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
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, VType: &pv, Mem: 256},
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VType: &pv, Mem: 512},
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
		ic := imagemetadata.ImageConstraint{
			CloudSpec: imagemetadata.CloudSpec{t.region, "ep"},
			Series:    "precise",
			Arches:    t.arches,
		}
		imageMeta, err := imagemetadata.GetLatestImageIdMetadata([]byte(jsonImagesContent), &ic)
		c.Assert(err, IsNil)
		var images []Image
		for _, imageMetadata := range imageMeta {
			im := *imageMetadata
			images = append(images, Image{
				Id:    im.Id,
				VType: im.VType,
				Arch:  im.Arch,
			})
		}
		spec, err := FindInstanceSpec(images, &InstanceConstraint{
			Series:         "precise",
			Region:         t.region,
			Arches:         t.arches,
			Constraints:    constraints.MustParse(t.constraints),
			DefaultImageId: t.defaultImageId,
		}, t.instanceTypes)
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
		image: Image{Arch: "amd64", VType: "pv"},
		itype: InstanceType{Arches: []string{"amd64"}, VType: &hvm},
	},
}

func (s *imageSuite) TestImageMatch(c *C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), Equals, t.match)
	}
}
