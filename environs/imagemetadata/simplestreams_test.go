// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"bytes"
	"flag"
	"net/http"
	"reflect"
	"strings"
	"testing"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	"gopkg.in/amz.v2/aws"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
)

var live = flag.Bool("live", false, "Include live simplestreams tests")
var vendor = flag.String("vendor", "", "The vendor representing the source of the simplestream data")

type liveTestData struct {
	baseURL        string
	requireSigned  bool
	validCloudSpec simplestreams.CloudSpec
}

var liveUrls = map[string]liveTestData{
	"ec2": {
		baseURL:        imagemetadata.DefaultBaseURL,
		requireSigned:  true,
		validCloudSpec: simplestreams.CloudSpec{"us-east-1", aws.Regions["us-east-1"].EC2Endpoint},
	},
	"canonistack": {
		baseURL:        "https://swift.canonistack.canonical.com/v1/AUTH_a48765cc0e864be980ee21ae26aaaed4/simplestreams/data",
		requireSigned:  false,
		validCloudSpec: simplestreams.CloudSpec{"lcy01", "https://keystone.canonistack.canonical.com:443/v1.0/"},
	},
}

func Test(t *testing.T) {
	if *live {
		if *vendor == "" {
			t.Fatal("missing vendor")
		}
		var ok bool
		var testData liveTestData
		if testData, ok = liveUrls[*vendor]; !ok {
			keys := reflect.ValueOf(liveUrls).MapKeys()
			t.Fatalf("Unknown vendor %s. Must be one of %s", *vendor, keys)
		}
		registerLiveSimpleStreamsTests(testData.baseURL, imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: testData.validCloudSpec,
			Series:    []string{"quantal"},
			Arches:    []string{"amd64"},
		}), testData.requireSigned)
	}
	registerSimpleStreamsTests()
	gc.TestingT(t)
}

func registerSimpleStreamsTests() {
	gc.Suite(&simplestreamsSuite{
		LocalLiveSimplestreamsSuite: sstesting.LocalLiveSimplestreamsSuite{
			Source: simplestreams.NewURLDataSource(
				"test roundtripper", "test:", utils.VerifySSLHostnames),
			RequireSigned:  false,
			DataType:       imagemetadata.ImageIds,
			StreamsVersion: imagemetadata.CurrentStreamsVersion,
			ValidConstraint: imagemetadata.NewImageConstraint(simplestreams.LookupParams{
				CloudSpec: simplestreams.CloudSpec{
					Region:   "us-east-1",
					Endpoint: "https://ec2.us-east-1.amazonaws.com",
				},
				Series: []string{"precise"},
				Arches: []string{"amd64", "arm"},
			}),
		},
	})
	gc.Suite(&signedSuite{})
}

func registerLiveSimpleStreamsTests(baseURL string, validImageConstraint simplestreams.LookupConstraint, requireSigned bool) {
	gc.Suite(&sstesting.LocalLiveSimplestreamsSuite{
		Source:          simplestreams.NewURLDataSource("test", baseURL, utils.VerifySSLHostnames),
		RequireSigned:   requireSigned,
		DataType:        imagemetadata.ImageIds,
		ValidConstraint: validImageConstraint,
	})
}

type simplestreamsSuite struct {
	sstesting.LocalLiveSimplestreamsSuite
	sstesting.TestDataSuite
}

func (s *simplestreamsSuite) SetUpSuite(c *gc.C) {
	s.LocalLiveSimplestreamsSuite.SetUpSuite(c)
	s.TestDataSuite.SetUpSuite(c)
}

func (s *simplestreamsSuite) TearDownSuite(c *gc.C) {
	s.TestDataSuite.TearDownSuite(c)
	s.LocalLiveSimplestreamsSuite.TearDownSuite(c)
}

var fetchTests = []struct {
	region  string
	version string
	arches  []string
	images  []*imagemetadata.ImageMetadata
}{
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64", "arm"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
			{
				Id:         "ami-442ea699",
				VirtType:   "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"arm"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea699",
				VirtType:   "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region:  "us-east-1",
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
	{
		version: "12.04",
		arches:  []string{"amd64"},
		images: []*imagemetadata.ImageMetadata{
			{
				Id:         "ami-26745463",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "au-east-2",
				Endpoint:   "https://somewhere-else",
				Storage:    "ebs",
			},
			{
				Id:         "ami-26745464",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "au-east-1",
				Endpoint:   "https://somewhere",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea674",
				VirtType:   "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:          "ami-442ea675",
				VirtType:    "hvm",
				Arch:        "amd64",
				RegionAlias: "uswest3",
				RegionName:  "us-west-3",
				Endpoint:    "https://ec2.us-west-3.amazonaws.com",
				Storage:     "ebs",
			},
			{
				Id:         "ami-442ea684",
				VirtType:   "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "https://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
}

func (s *simplestreamsSuite) TestFetch(c *gc.C) {
	for i, t := range fetchTests {
		c.Logf("test %d", i)
		cloudSpec := simplestreams.CloudSpec{t.region, "https://ec2.us-east-1.amazonaws.com"}
		if t.region == "" {
			cloudSpec = simplestreams.EmptyCloudSpec
		}
		imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
			CloudSpec: cloudSpec,
			Series:    []string{"precise"},
			Arches:    t.arches,
		})
		// Add invalid datasource and check later that resolveInfo is correct.
		invalidSource := simplestreams.NewURLDataSource("invalid", "file://invalid", utils.VerifySSLHostnames)
		images, resolveInfo, err := imagemetadata.Fetch(
			[]simplestreams.DataSource{invalidSource, s.Source}, imageConstraint, s.RequireSigned)
		if !c.Check(err, jc.ErrorIsNil) {
			continue
		}
		for _, testImage := range t.images {
			testImage.Version = t.version
		}
		c.Check(images, gc.DeepEquals, t.images)
		c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
			Source:    "test roundtripper",
			Signed:    s.RequireSigned,
			IndexURL:  "test:/streams/v1/index.json",
			MirrorURL: "",
		})
	}
}

type productSpecSuite struct{}

var _ = gc.Suite(&productSpecSuite{})

func (s *productSpecSuite) TestIdWithDefaultStream(c *gc.C) {
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64"},
	})
	for _, stream := range []string{"", "released"} {
		imageConstraint.Stream = stream
		ids, err := imageConstraint.ProductIds()
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(ids, gc.DeepEquals, []string{"com.ubuntu.cloud:server:12.04:amd64"})
	}
}

func (s *productSpecSuite) TestId(c *gc.C) {
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64"},
		Stream: "daily",
	})
	ids, err := imageConstraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, gc.DeepEquals, []string{"com.ubuntu.cloud.daily:server:12.04:amd64"})
}

func (s *productSpecSuite) TestIdMultiArch(c *gc.C) {
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		Series: []string{"precise"},
		Arches: []string{"amd64", "i386"},
		Stream: "daily",
	})
	ids, err := imageConstraint.ProductIds()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ids, gc.DeepEquals, []string{
		"com.ubuntu.cloud.daily:server:12.04:amd64",
		"com.ubuntu.cloud.daily:server:12.04:i386"})
}

type signedSuite struct {
	origKey string
}

var testRoundTripper *jujutest.ProxyRoundTripper

func init() {
	testRoundTripper = &jujutest.ProxyRoundTripper{}
	testRoundTripper.RegisterForScheme("signedtest")
}

func (s *signedSuite) SetUpSuite(c *gc.C) {
	var imageData = map[string]string{
		"/unsigned/streams/v1/index.json":          unsignedIndex,
		"/unsigned/streams/v1/image_metadata.json": unsignedProduct,
	}

	// Set up some signed data from the unsigned data.
	// Overwrite the product path to use the sjson suffix.
	rawUnsignedIndex := strings.Replace(
		unsignedIndex, "streams/v1/image_metadata.json", "streams/v1/image_metadata.sjson", -1)
	r := bytes.NewReader([]byte(rawUnsignedIndex))
	signedData, err := simplestreams.Encode(
		r, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	c.Assert(err, jc.ErrorIsNil)
	imageData["/signed/streams/v1/index.sjson"] = string(signedData)

	// Replace the image id in the unsigned data with a different one so we can test that the right
	// image id is used.
	rawUnsignedProduct := strings.Replace(
		unsignedProduct, "ami-26745463", "ami-123456", -1)
	r = bytes.NewReader([]byte(rawUnsignedProduct))
	signedData, err = simplestreams.Encode(
		r, sstesting.SignedMetadataPrivateKey, sstesting.PrivateKeyPassphrase)
	c.Assert(err, jc.ErrorIsNil)
	imageData["/signed/streams/v1/image_metadata.sjson"] = string(signedData)
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(
		imageData, map[string]int{"signedtest://unauth": http.StatusUnauthorized})
	s.origKey = imagemetadata.SetSigningPublicKey(sstesting.SignedMetadataPublicKey)
}

func (s *signedSuite) TearDownSuite(c *gc.C) {
	testRoundTripper.Sub = nil
	imagemetadata.SetSigningPublicKey(s.origKey)
}

func (s *signedSuite) TestSignedImageMetadata(c *gc.C) {
	signedSource := simplestreams.NewURLDataSource("test", "signedtest://host/signed", utils.VerifySSLHostnames)
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
		Series:    []string{"precise"},
		Arches:    []string{"amd64"},
	})
	images, resolveInfo, err := imagemetadata.Fetch([]simplestreams.DataSource{signedSource}, imageConstraint, true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(images), gc.Equals, 1)
	c.Assert(images[0].Id, gc.Equals, "ami-123456")
	c.Check(resolveInfo, gc.DeepEquals, &simplestreams.ResolveInfo{
		Source:    "test",
		Signed:    true,
		IndexURL:  "signedtest://host/signed/streams/v1/index.sjson",
		MirrorURL: "",
	})
}

func (s *signedSuite) TestSignedImageMetadataInvalidSignature(c *gc.C) {
	signedSource := simplestreams.NewURLDataSource("test", "signedtest://host/signed", utils.VerifySSLHostnames)
	imageConstraint := imagemetadata.NewImageConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"us-east-1", "https://ec2.us-east-1.amazonaws.com"},
		Series:    []string{"precise"},
		Arches:    []string{"amd64"},
	})
	imagemetadata.SetSigningPublicKey(s.origKey)
	_, _, err := imagemetadata.Fetch([]simplestreams.DataSource{signedSource}, imageConstraint, true)
	c.Assert(err, gc.ErrorMatches, "cannot read index data.*")
}

var unsignedIndex = `
{
 "index": {
  "com.ubuntu.cloud:released:precise": {
   "updated": "Wed, 01 May 2013 13:31:26 +0000",
   "clouds": [
	{
	 "region": "us-east-1",
	 "endpoint": "https://ec2.us-east-1.amazonaws.com"
	}
   ],
   "cloudname": "aws",
   "datatype": "image-ids",
   "format": "products:1.0",
   "products": [
	"com.ubuntu.cloud:server:12.04:amd64"
   ],
   "path": "streams/v1/image_metadata.json"
  }
 },
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "format": "index:1.0"
}
`
var unsignedProduct = `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
  "com.ubuntu.cloud:server:12.04:amd64": {
   "release": "precise",
   "version": "12.04",
   "arch": "amd64",
   "region": "us-east-1",
   "endpoint": "https://somewhere",
   "versions": {
    "20121218": {
     "region": "us-east-1",
     "endpoint": "https://somewhere-else",
     "items": {
      "usww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745463"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
     "label": "release"
    }
   }
  }
 },
 "format": "products:1.0"
}
`
