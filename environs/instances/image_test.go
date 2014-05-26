// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"testing"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/constraints"
	"launchpad.net/juju-core/environs/imagemetadata"
	"launchpad.net/juju-core/environs/simplestreams"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/utils"
)

type imageSuite struct {
	coretesting.BaseSuite
}

func Test(t *testing.T) {
	gc.TestingT(t)
}

var _ = gc.Suite(&imageSuite{})

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
           },
           "test2he": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "test",
             "id": "ami-00000036"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-server-20121118",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:armhf": {
     "release": "precise",
     "version": "12.04",
     "arch": "armhf",
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
         "pubname": "ubuntu-precise-12.04-armhf-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:i386": {
     "release": "precise",
     "version": "12.04",
     "arch": "i386",
     "versions": {
       "20121218": {
         "items": {
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-b79b09b6"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-b79b09b7"
           }
         },
         "pubname": "ubuntu-precise-12.04-i386-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:ppc64": {
     "release": "precise",
     "version": "12.04",
     "arch": "ppc64",
     "versions": {
       "20121218": {
         "items": {
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-b79b09b8"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "test",
             "id": "ami-b79b09b9"
           }
         },
         "pubname": "ubuntu-precise-12.04-ppc64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud.daily:server:12.04:amd64": {
     "release": "precise",
     "version": "12.04",
     "arch": "amd64",
     "versions": {
       "20121218": {
         "items": {
           "apne1pe": {
             "root_store": "ebs",
             "virt": "pv",
             "region": "ap-northeast-1",
             "id": "ami-10000026"
           },
           "test1pe": {
             "root_store": "ebs",
             "virt": "hvm",
             "region": "test",
             "id": "ami-10000035"
           }
         },
         "pubname": "ubuntu-precise-12.04-amd64-daily-20121218",
         "label": "release"
       }
     }
   }
 },
 "format": "products:1.0"
}
`

type instanceSpecTestParams struct {
	desc             string
	region           string
	arches           []string
	stream           string
	constraints      string
	instanceTypes    []InstanceType
	imageId          string
	instanceTypeId   string
	instanceTypeName string
	err              string
}

func (p *instanceSpecTestParams) init() {
	if p.arches == nil {
		p.arches = []string{"amd64", "armhf"}
	}
	if p.instanceTypes == nil {
		p.instanceTypes = []InstanceType{{Id: "1", Name: "it-1", Arches: []string{"amd64", "armhf"}}}
		p.instanceTypeId = "1"
		p.instanceTypeName = "it-1"
	}
}

var pv = "pv"
var findInstanceSpecTests = []instanceSpecTestParams{
	{
		desc:    "image exists in metadata",
		region:  "test",
		imageId: "ami-00000033",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "prefer amd64 over i386",
		region:  "test",
		imageId: "ami-00000033",
		arches:  []string{"amd64", "i386"},
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"i386", "amd64"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "prefer armhf over i386 (first alphabetical wins)",
		region:  "test",
		imageId: "ami-00000034",
		arches:  []string{"armhf", "i386"},
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"armhf", "i386"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "prefer ppc64 over i386 (64-bit trumps 32-bit, regardless of alphabetical order)",
		region:  "test",
		imageId: "ami-b79b09b9",
		arches:  []string{"ppc64", "i386"},
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"i386", "ppc64"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "prefer amd64 over arm64 (first 64-bit alphabetical wins)",
		region:  "test",
		imageId: "ami-00000033",
		arches:  []string{"arm64", "amd64"},
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"arm64", "amd64"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "explicit release stream",
		region:  "test",
		stream:  "released",
		imageId: "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:    "non-release stream",
		region:  "test",
		stream:  "daily",
		imageId: "ami-10000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:    "multiple images exists in metadata, use most recent",
		region:  "test",
		imageId: "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:        "empty instance type constraint",
		region:      "test",
		constraints: "instance-type=",
		imageId:     "ami-00000033",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:        "use instance type constraint",
		region:      "test",
		constraints: "instance-type=it-1",
		imageId:     "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 512, CpuCores: 2},
			{Id: "2", Name: "it-2", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 1024, CpuCores: 2},
		},
	},
	{
		desc:        "instance type constraint, no matching instance types",
		region:      "test",
		constraints: "instance-type=it-10",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arches: []string{"amd64"}, VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
		err: `invalid instance type "it-10"`,
	},
	{
		desc:   "no image exists in metadata",
		region: "invalid-region",
		err:    `no "precise" images in invalid-region with arches \[amd64 armhf\]`,
	},
	{
		desc:          "no valid instance types",
		region:        "test",
		instanceTypes: []InstanceType{},
		err:           `no instance types in test matching constraints ""`,
	},
	{
		desc:          "no compatible instance types",
		region:        "arm-only",
		instanceTypes: []InstanceType{{Id: "1", Name: "it-1", Arches: []string{"amd64"}, Mem: 2048}},
		err:           `no "precise" images in arm-only matching instance types \[it-1\]`,
	},
}

func (s *imageSuite) TestFindInstanceSpec(c *gc.C) {
	for _, t := range findInstanceSpecTests {
		c.Logf("test: %v", t.desc)
		t.init()
		cons := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: simplestreams.CloudSpec{t.region, "ep"},
			Series:    []string{"precise"},
			Arches:    t.arches,
			Stream:    t.stream,
		})
		imageMeta, err := imagemetadata.GetLatestImageIdMetadata(
			[]byte(jsonImagesContent),
			simplestreams.NewURLDataSource("test", "some-url", utils.VerifySSLHostnames), cons)
		c.Assert(err, gc.IsNil)
		var images []Image
		for _, imageMetadata := range imageMeta {
			im := *imageMetadata
			images = append(images, Image{
				Id:       im.Id,
				VirtType: im.VirtType,
				Arch:     im.Arch,
			})
		}
		imageCons := constraints.MustParse(t.constraints)
		spec, err := FindInstanceSpec(images, &InstanceConstraint{
			Series:      "precise",
			Region:      t.region,
			Arches:      t.arches,
			Constraints: imageCons,
		}, t.instanceTypes)
		if t.err != "" {
			c.Check(err, gc.ErrorMatches, t.err)
			continue
		} else {
			if !c.Check(err, gc.IsNil) {
				continue
			}
			c.Check(spec.Image.Id, gc.Equals, t.imageId)
			if len(t.instanceTypes) == 1 {
				c.Check(spec.InstanceType, gc.DeepEquals, t.instanceTypes[0])
			}
			if imageCons.HasInstanceType() {
				c.Assert(spec.InstanceType.Name, gc.Equals, *imageCons.InstanceType)
			}
		}
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
		itype: InstanceType{Arches: []string{"amd64", "armhf"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", VirtType: hvm},
		itype: InstanceType{Arches: []string{"amd64"}, VirtType: &hvm},
		match: true,
	}, {
		image: Image{Arch: "armhf"},
		itype: InstanceType{Arches: []string{"amd64"}},
	}, {
		image: Image{Arch: "amd64", VirtType: hvm},
		itype: InstanceType{Arches: []string{"amd64"}},
		match: true,
	}, {
		image: Image{Arch: "amd64", VirtType: "pv"},
		itype: InstanceType{Arches: []string{"amd64"}, VirtType: &hvm},
	},
}

func (s *imageSuite) TestImageMatch(c *gc.C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), gc.Equals, t.match)
	}
}

func (*imageSuite) TestImageMetadataToImagesAcceptsNil(c *gc.C) {
	c.Check(ImageMetadataToImages(nil), gc.HasLen, 0)
}

func (*imageSuite) TestImageMetadataToImagesConvertsSelectMetadata(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{
			Id:          "id",
			Storage:     "storage-is-ignored",
			VirtType:    "vtype",
			Arch:        "arch",
			RegionAlias: "region-alias-is-ignored",
			RegionName:  "region-name-is-ignored",
			Endpoint:    "endpoint-is-ignored",
		},
	}
	expectation := []Image{
		{
			Id:       "id",
			VirtType: "vtype",
			Arch:     "arch",
		},
	}
	c.Check(ImageMetadataToImages(input), gc.DeepEquals, expectation)
}

func (*imageSuite) TestImageMetadataToImagesMaintainsOrdering(c *gc.C) {
	input := []*imagemetadata.ImageMetadata{
		{Id: "one", Arch: "Z80"},
		{Id: "two", Arch: "i386"},
		{Id: "three", Arch: "amd64"},
	}
	expectation := []Image{
		{Id: "one", Arch: "Z80"},
		{Id: "two", Arch: "i386"},
		{Id: "three", Arch: "amd64"},
	}
	c.Check(ImageMetadataToImages(input), gc.DeepEquals, expectation)
}
