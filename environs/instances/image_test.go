// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instances

import (
	"testing"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	coretesting "github.com/juju/juju/internal/testing"
)

type imageSuite struct {
	coretesting.BaseSuite
}

func Test(t *testing.T) {
	tc.TestingT(t)
}

var _ = tc.Suite(&imageSuite{})

var jsonImagesContent = `
{
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
   "com.ubuntu.cloud:server:12.04:amd64": {
     "release": "12.04",
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
   "com.ubuntu.cloud:server:12.04:arm64": {
     "release": "12.04",
     "version": "12.04",
     "arch": "arm64",
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
         "pubname": "ubuntu-precise-12.04-arm64-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud:server:12.04:i386": {
     "release": "12.04",
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
   "com.ubuntu.cloud:server:12.04:ppc64el": {
     "release": "12.04",
     "version": "12.04",
     "arch": "ppc64el",
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
         "pubname": "ubuntu-precise-12.04-ppc64el-server-20121218",
         "label": "release"
       }
     }
   },
   "com.ubuntu.cloud.daily:server:12.04:amd64": {
     "release": "12.04",
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
	arch             string
	stream           string
	constraints      string
	instanceTypes    []InstanceType
	imageId          string
	instanceTypeId   string
	instanceTypeName string
	err              string
}

func (p *instanceSpecTestParams) init() {
	if p.arch == "" {
		p.arch = "amd64"
	}
	if p.instanceTypes == nil {
		p.instanceTypes = []InstanceType{{Id: "1", Name: "it-1", Arch: "amd64"}}
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
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:    "explicit release stream",
		region:  "test",
		stream:  "released",
		imageId: "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:    "non-release stream",
		region:  "test",
		stream:  "daily",
		imageId: "ami-10000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:    "multiple images exists in metadata, use most recent",
		region:  "test",
		imageId: "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
	},
	{
		desc:        "empty instance type constraint",
		region:      "test",
		constraints: "instance-type=",
		imageId:     "ami-00000033",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &pv, Mem: 512},
		},
	},
	{
		desc:        "use instance type constraint",
		region:      "test",
		constraints: "instance-type=it-1",
		imageId:     "ami-00000035",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &hvm, Mem: 512, CpuCores: 2},
			{Id: "2", Name: "it-2", Arch: "amd64", VirtType: &hvm, Mem: 1024, CpuCores: 2},
		},
	},
	{
		desc:    "use instance type non amd64 constraint",
		region:  "test",
		arch:    "ppc64el",
		imageId: "ami-b79b09b9",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &pv, Mem: 4096, CpuCores: 2, Cost: 1},
			{Id: "2", Name: "it-2", Arch: "ppc64el", VirtType: &pv, Mem: 4096, CpuCores: 2, Cost: 2},
		},
	},
	{
		desc:        "instance type constraint, no matching instance types",
		region:      "test",
		constraints: "instance-type=it-10",
		instanceTypes: []InstanceType{
			{Id: "1", Name: "it-1", Arch: "amd64", VirtType: &hvm, Mem: 512, CpuCores: 2},
		},
		err: `no instance types in test matching constraints "arch=amd64 instance-type=it-10"`,
	},
	{
		desc:   "no image exists in metadata",
		region: "invalid-region",
		err:    `no metadata for "ubuntu@12.04" images in invalid-region with arch amd64`,
	},
	{
		desc:          "no valid instance types",
		region:        "test",
		instanceTypes: []InstanceType{},
		err:           `no instance types in test matching constraints "arch=amd64"`,
	},
	{
		desc:          "no compatible instance types",
		region:        "arm-only",
		instanceTypes: []InstanceType{{Id: "1", Name: "it-1", Arch: "amd64", Mem: 2048}},
		err:           `no "ubuntu@12.04" images in arm-only matching instance types \[it-1\]`,
	},
}

func (s *imageSuite) TestFindInstanceSpec(c *tc.C) {
	for _, t := range findInstanceSpecTests {
		c.Logf("test: %v", t.desc)
		t.init()
		cons, err := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: simplestreams.CloudSpec{t.region, "ep"},
			Releases:  []string{"12.04"},
			Stream:    t.stream,
		})
		c.Assert(err, jc.ErrorIsNil)
		dataSource := simplestreams.NewDataSource(simplestreams.Config{
			Description:          "test",
			BaseURL:              "some-url",
			HostnameVerification: true,
			Priority:             simplestreams.DEFAULT_CLOUD_DATA,
		})
		imageMeta, err := imagemetadata.GetLatestImageIdMetadata([]byte(jsonImagesContent), dataSource, cons)
		c.Assert(err, jc.ErrorIsNil)
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
			Base:        corebase.MakeDefaultBase("ubuntu", "12.04"),
			Region:      t.region,
			Arch:        t.arch,
			Constraints: imageCons,
		}, t.instanceTypes)
		if t.err != "" {
			c.Check(err, tc.ErrorMatches, t.err)
			continue
		} else {
			if !c.Check(err, jc.ErrorIsNil) {
				continue
			}
			c.Check(spec.Image.Id, tc.Equals, t.imageId)
			if len(t.instanceTypes) == 1 {
				c.Check(spec.InstanceType, tc.DeepEquals, t.instanceTypes[0])
			}
			if imageCons.HasInstanceType() {
				c.Assert(spec.InstanceType.Name, tc.Equals, *imageCons.InstanceType)
			}
		}
	}
}

var imageMatchtests = []struct {
	image Image
	itype InstanceType
	match imageMatch
}{
	{
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arch: "amd64"},
		match: exactMatch,
	}, {
		image: Image{Arch: "amd64"},
		itype: InstanceType{Arch: "amd64"},
		match: exactMatch,
	}, {
		image: Image{Arch: "amd64", VirtType: hvm},
		itype: InstanceType{Arch: "amd64", VirtType: &hvm},
		match: exactMatch,
	}, {
		image: Image{Arch: "arm64"},
		itype: InstanceType{Arch: "amd64"},
	}, {
		image: Image{Arch: "amd64", VirtType: hvm},
		itype: InstanceType{Arch: "amd64"},
		match: exactMatch,
	}, {
		image: Image{Arch: "amd64"}, // no known virt type
		itype: InstanceType{Arch: "amd64", VirtType: &hvm},
		match: partialMatch,
	}, {
		image: Image{Arch: "amd64", VirtType: "pv"},
		itype: InstanceType{Arch: "amd64", VirtType: &hvm},
		match: nonMatch,
	},
}

func (s *imageSuite) TestImageMatch(c *tc.C) {
	for i, t := range imageMatchtests {
		c.Logf("test %d", i)
		c.Check(t.image.match(t.itype), tc.Equals, t.match)
	}
}

func (*imageSuite) TestImageMetadataToImagesAcceptsNil(c *tc.C) {
	c.Check(ImageMetadataToImages(nil), tc.HasLen, 0)
}

func (*imageSuite) TestImageMetadataToImagesConvertsSelectMetadata(c *tc.C) {
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
	c.Check(ImageMetadataToImages(input), tc.DeepEquals, expectation)
}

func (*imageSuite) TestImageMetadataToImagesMaintainsOrdering(c *tc.C) {
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
	c.Check(ImageMetadataToImages(input), tc.DeepEquals, expectation)
}

func (*imageSuite) TestInstanceConstraintString(c *tc.C) {
	imageCons := constraints.MustParse("mem=4G")
	ic := &InstanceConstraint{
		Base:        corebase.MakeDefaultBase("ubuntu", "12.04"),
		Region:      "region",
		Arch:        "amd64",
		Constraints: imageCons,
	}
	c.Assert(
		ic.String(), tc.Equals,
		"{region: region, base: ubuntu@12.04, arch: amd64, constraints: mem=4096M, storage: []}")

	ic.Storage = []string{"ebs", "ssd"}
	c.Assert(
		ic.String(), tc.Equals,
		"{region: region, base: ubuntu@12.04, arch: amd64, constraints: mem=4096M, storage: [ebs ssd]}")
}
