package imagemetadata

import (
	"flag"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/jujutest"
	coretesting "launchpad.net/juju-core/testing"
	"net/http"
	"reflect"
	"testing"
)

var live = flag.Bool("live", false, "Include live simplestreams tests")
var vendor = flag.String("vendor", "", "The vendor representing the source of the simplestream data")

type liveTestData struct {
	baseURL        string
	validCloudSpec CloudSpec
}

var liveUrls = map[string]liveTestData{
	"ec2": {
		baseURL:        DefaultBaseURL,
		validCloudSpec: CloudSpec{"us-east-1", "http://ec2.us-east-1.amazonaws.com"},
	},
	"canonistack": {
		baseURL:        "https://swift.canonistack.canonical.com/v1/AUTH_a48765cc0e864be980ee21ae26aaaed4/simplestreams/data",
		validCloudSpec: CloudSpec{"lcy01", "https://keystone.canonistack.canonical.com:443/v2.0/"},
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
		registerLiveSimpleStreamsTests(testData.baseURL, ImageConstraint{
			CloudSpec: testData.validCloudSpec,
			Release:   "quantal",
			Arches:    []string{"amd64"},
		})
	}
	registerSimpleStreamsTests()
	TestingT(t)
}

var testRoundTripper = &jujutest.ProxyRoundTripper{}

func init() {
	// Prepare mock http transport for overriding metadata and images output in tests
	http.DefaultTransport.(*http.Transport).RegisterProtocol("test", testRoundTripper)
}

var imageData = []jujutest.FileContent{
	{
		"/streams/v1/index.json", `
		{
		 "index": {
		  "com.ubuntu.cloud:released:precise": {
		   "updated": "Wed, 01 May 2013 13:31:26 +0000",
		   "clouds": [
			{
			 "region": "us-east-1",
			 "endpoint": "http://ec2.us-east-1.amazonaws.com"
			}
		   ],
		   "cloudname": "aws",
		   "datatype": "image-ids",
		   "format": "products:1.0",
		   "products": [
			"com.ubuntu.cloud:server:12.04:amd64",
			"com.ubuntu.cloud:server:12.04:arm"
		   ],
		   "path": "streams/v1/image_metadata.json"
		  },
		  "com.ubuntu.cloud:released:raring": {
		   "updated": "Wed, 01 May 2013 13:31:26 +0000",
		   "clouds": [
			{
			 "region": "us-east-1",
			 "endpoint": "http://ec2.us-east-1.amazonaws.com"
			}
		   ],
		   "cloudname": "aws",
		   "datatype": "image-ids",
		   "format": "products:1.0",
		   "products": [
			"com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "path": "streams/v1/raring_metadata.json"
		  },
		  "com.ubuntu.cloud:released:download": {
		   "datatype": "image-downloads",
		   "path": "streams/v1/com.ubuntu.cloud:released:download.json",
		   "updated": "Wed, 01 May 2013 13:30:37 +0000",
		   "products": [
			"com.ubuntu.cloud:server:12.10:amd64",
			"com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "format": "products:1.0"
		  }
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`}, {
		"/streams/v1/image_metadata.json", `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
  "com.ubuntu.cloud:server:12.04:amd64": {
   "release": "precise",
   "version": "12.04",
   "arch": "amd64",
   "region": "au-east-1",
   "endpoint": "http://somewhere",
   "versions": {
    "20121218": {
     "region": "au-east-2",
     "endpoint": "http://somewhere-else",
     "items": {
      "usww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745463"
      },
      "usww2he": {
       "root_store": "ebs",
       "virt": "hvm",
       "id": "ami-442ea674",
       "region": "us-east-1",
       "endpoint": "http://ec2.us-east-1.amazonaws.com"
      },
      "usww3he": {
       "root_store": "ebs",
       "virt": "hvm",
       "crsn": "uswest3",
       "id": "ami-442ea675"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
     "label": "release"
    },
    "20111111": {
     "items": {
      "usww3pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-26745464"
      },
      "usww2pe": {
       "root_store": "instance",
       "virt": "pv",
       "id": "ami-442ea684",
       "region": "us-east-1",
       "endpoint": "http://ec2.us-east-1.amazonaws.com"
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20111111",
     "label": "release"
    }
   }
  },
  "com.ubuntu.cloud:server:12.04:arm": {
   "release": "precise",
   "version": "12.04",
   "arch": "arm",
   "region": "us-east-1",
   "endpoint": "http://ec2.us-east-1.amazonaws.com",
   "versions": {
    "20121219": {
     "items": {
      "usww2he": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-442ea699"
      }
     },
     "pubname": "ubuntu-precise-12.04-arm-server-20121219",
     "label": "release"
    }
   }
  }
 },
 "_aliases": {
  "crsn": {
   "uswest3": {
    "region": "us-west-3",
    "endpoint": "http://ec2.us-west-3.amazonaws.com"
   }
  }
 },
 "format": "products:1.0"
}
`},
}

func registerSimpleStreamsTests() {
	Suite(&simplestreamsSuite{
		liveSimplestreamsSuite: liveSimplestreamsSuite{
			baseURL: "test:",
			validImageConstraint: NewImageConstraint(
				"us-east-1", "http://ec2.us-east-1.amazonaws.com", "precise", []string{"amd64", "arm"}, ""),
		},
	})
}

func registerLiveSimpleStreamsTests(baseURL string, validImageConstraint ImageConstraint) {
	Suite(&liveSimplestreamsSuite{
		baseURL:              baseURL,
		validImageConstraint: validImageConstraint,
	})
}

type simplestreamsSuite struct {
	liveSimplestreamsSuite
}

type liveSimplestreamsSuite struct {
	coretesting.LoggingSuite
	baseURL              string
	validImageConstraint ImageConstraint
}

func (s *liveSimplestreamsSuite) SetUpSuite(c *C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *liveSimplestreamsSuite) TearDownSuite(c *C) {
	s.LoggingSuite.TearDownSuite(c)
}

func (s *simplestreamsSuite) SetUpSuite(c *C) {
	s.liveSimplestreamsSuite.SetUpSuite(c)
	testRoundTripper.Sub = jujutest.NewVirtualRoundTripper(imageData)
}

func (s *simplestreamsSuite) TearDownSuite(c *C) {
	testRoundTripper.Sub = nil
	s.liveSimplestreamsSuite.TearDownSuite(c)
}

const (
	index_v1   = "index:1.0"
	product_v1 = "products:1.0"
)

func (s *liveSimplestreamsSuite) TestGetIndex(c *C) {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	c.Assert(indexRef.Format, Equals, index_v1)
	c.Assert(indexRef.baseURL, Equals, s.baseURL)
	c.Assert(len(indexRef.Indexes) > 0, Equals, true)
}

func (s *liveSimplestreamsSuite) TestGetIndexWrongFormat(c *C) {
	_, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, "bad")
	c.Assert(err, NotNil)
}

func (s *liveSimplestreamsSuite) TestGetImageIdsPathExists(c *C) {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	path, err := indexRef.getImageIdsPath(&s.validImageConstraint)
	c.Assert(err, IsNil)
	c.Assert(path, Not(Equals), "")
}

func (s *liveSimplestreamsSuite) TestGetImageIdsPathInvalidCloudSpec(c *C) {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	ic := ImageConstraint{
		CloudSpec: CloudSpec{"bad", "spec"},
	}
	_, err = indexRef.getImageIdsPath(&ic)
	c.Assert(err, NotNil)
}

func (s *liveSimplestreamsSuite) TestGetImageIdsPathInvalidProductSpec(c *C) {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	ic := ImageConstraint{
		CloudSpec: s.validImageConstraint.CloudSpec,
		Release:   "precise",
		Arches:    []string{"bad"},
		Stream:    "spec",
	}
	_, err = indexRef.getImageIdsPath(&ic)
	c.Assert(err, NotNil)
}

func (s *simplestreamsSuite) TestGetImageIdsPath(c *C) {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	path, err := indexRef.getImageIdsPath(&s.validImageConstraint)
	c.Assert(err, IsNil)
	c.Assert(path, Equals, "streams/v1/image_metadata.json")
}

func (s *liveSimplestreamsSuite) assertGetMetadata(c *C) *cloudImageMetadata {
	indexRef, err := getIndexWithFormat(s.baseURL, DefaultIndexPath, index_v1)
	c.Assert(err, IsNil)
	metadata, err := indexRef.getCloudMetadataWithFormat(&s.validImageConstraint, product_v1)
	c.Assert(err, IsNil)
	c.Assert(metadata.Format, Equals, product_v1)
	c.Assert(len(metadata.Products) > 0, Equals, true)
	return metadata
}

func (s *liveSimplestreamsSuite) TestGetCloudMetadataWithFormat(c *C) {
	s.assertGetMetadata(c)
}

func (s *liveSimplestreamsSuite) TestGetImageIdMetadataExists(c *C) {
	im, err := GetImageIdMetadata([]string{s.baseURL}, DefaultIndexPath, &s.validImageConstraint)
	c.Assert(err, IsNil)
	c.Assert(len(im) > 0, Equals, true)
}

func (s *liveSimplestreamsSuite) TestGetImageIdMetadataMultipleBaseURLsExists(c *C) {
	im, err := GetImageIdMetadata([]string{"http://bad", s.baseURL}, DefaultIndexPath, &s.validImageConstraint)
	c.Assert(err, IsNil)
	c.Assert(len(im) > 0, Equals, true)
}

func (s *liveSimplestreamsSuite) assertGetImageCollections(c *C, version string) *imageCollection {
	metadata := s.assertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Images[version]
	return ic
}

func (s *simplestreamsSuite) TestMetadataCatalog(c *C) {
	metadata := s.assertGetMetadata(c)
	c.Check(len(metadata.Products), Equals, 2)
	c.Check(len(metadata.Aliases), Equals, 1)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	c.Check(len(metadataCatalog.Images), Equals, 2)
	c.Check(metadataCatalog.Release, Equals, "precise")
	c.Check(metadataCatalog.Version, Equals, "12.04")
	c.Check(metadataCatalog.Arch, Equals, "amd64")
	c.Check(metadataCatalog.RegionName, Equals, "au-east-1")
	c.Check(metadataCatalog.Endpoint, Equals, "http://somewhere")
	c.Check(len(metadataCatalog.Images) > 0, Equals, true)
}

func (s *simplestreamsSuite) TestImageCollection(c *C) {
	ic := s.assertGetImageCollections(c, "20121218")
	c.Check(ic.RegionName, Equals, "au-east-2")
	c.Check(ic.Endpoint, Equals, "http://somewhere-else")
	c.Assert(len(ic.Images) > 0, Equals, true)
	im := ic.Images["usww2he"]
	c.Check(im.Id, Equals, "ami-442ea674")
	c.Check(im.Storage, Equals, "ebs")
	c.Check(im.VType, Equals, "hvm")
	c.Check(im.RegionName, Equals, "us-east-1")
	c.Check(im.Endpoint, Equals, "http://ec2.us-east-1.amazonaws.com")
}

func (s *simplestreamsSuite) TestImageMetadataDenormalisationFromCollection(c *C) {
	ic := s.assertGetImageCollections(c, "20121218")
	im := ic.Images["usww1pe"]
	c.Check(im.RegionName, Equals, ic.RegionName)
	c.Check(im.Endpoint, Equals, ic.Endpoint)
}

func (s *simplestreamsSuite) TestImageMetadataDenormalisationFromCatalog(c *C) {
	metadata := s.assertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Images["20111111"]
	im := ic.Images["usww3pe"]
	c.Check(im.RegionName, Equals, metadataCatalog.RegionName)
	c.Check(im.Endpoint, Equals, metadataCatalog.Endpoint)
}

func (s *simplestreamsSuite) TestImageMetadataDealiasing(c *C) {
	metadata := s.assertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Images["20121218"]
	im := ic.Images["usww3he"]
	c.Check(im.RegionName, Equals, "us-west-3")
	c.Check(im.Endpoint, Equals, "http://ec2.us-west-3.amazonaws.com")
}

type productSpecSuite struct{}

var _ = Suite(&productSpecSuite{})

func (s *productSpecSuite) TestIdWithDefaultStream(c *C) {
	imageConstraint := NewImageConstraint("region", "ep", "precise", []string{"amd64"}, "")
	ids, err := imageConstraint.Ids()
	c.Assert(err, IsNil)
	c.Assert(ids, DeepEquals, []string{"com.ubuntu.cloud:server:12.04:amd64"})
	c.Assert(imageConstraint.cachedIds, DeepEquals, ids)
}

func (s *productSpecSuite) TestId(c *C) {
	imageConstraint := NewImageConstraint("region", "ep", "precise", []string{"amd64"}, "daily")
	ids, err := imageConstraint.Ids()
	c.Assert(err, IsNil)
	c.Assert(ids, DeepEquals, []string{"com.ubuntu.cloud.daily:server:12.04:amd64"})
	c.Assert(imageConstraint.cachedIds, DeepEquals, ids)
}

func (s *productSpecSuite) TestIdMultiArch(c *C) {
	imageConstraint := NewImageConstraint("region", "ep", "precise", []string{"amd64", "i386"}, "daily")
	ids, err := imageConstraint.Ids()
	c.Assert(err, IsNil)
	c.Assert(ids, DeepEquals, []string{
		"com.ubuntu.cloud.daily:server:12.04:amd64",
		"com.ubuntu.cloud.daily:server:12.04:i386"})
	c.Assert(imageConstraint.cachedIds, DeepEquals, ids)
}

func (s *productSpecSuite) TestIdWithNonDefaultRelease(c *C) {
	imageConstraint := NewImageConstraint("region", "ep", "lucid", []string{"amd64"}, "daily")
	ids, err := imageConstraint.Ids()
	c.Assert(err, IsNil)
	c.Assert(ids, DeepEquals, []string{"com.ubuntu.cloud.daily:server:10.04:amd64"})
	c.Assert(imageConstraint.cachedIds, DeepEquals, ids)
}

var ebs = "ebs"
var getImageIdMetadataTests = []struct {
	region  string
	series  string
	arches  []string
	images  []*ImageMetadata
	storage *string
}{
	{
		region: "us-east-1",
		series: "precise",
		arches: []string{"amd64", "arm"},
		images: []*ImageMetadata{
			{
				Id:         "ami-442ea674",
				VType:      "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VType:      "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
			{
				Id:         "ami-442ea699",
				VType:      "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region: "us-east-1",
		series: "precise",
		arches: []string{"amd64"},
		images: []*ImageMetadata{
			{
				Id:         "ami-442ea674",
				VType:      "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
			{
				Id:         "ami-442ea684",
				VType:      "pv",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "instance",
			},
		},
	},
	{
		region: "us-east-1",
		series: "precise",
		arches: []string{"arm"},
		images: []*ImageMetadata{
			{
				Id:         "ami-442ea699",
				VType:      "pv",
				Arch:       "arm",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
	{
		region:  "us-east-1",
		series:  "precise",
		arches:  []string{"amd64"},
		storage: &ebs,
		images: []*ImageMetadata{
			{
				Id:         "ami-442ea674",
				VType:      "hvm",
				Arch:       "amd64",
				RegionName: "us-east-1",
				Endpoint:   "http://ec2.us-east-1.amazonaws.com",
				Storage:    "ebs",
			},
		},
	},
}

func (s *simplestreamsSuite) TestGetImageIdMetadata(c *C) {
	for i, t := range getImageIdMetadataTests {
		c.Logf("test %d", i)
		imageConstraint := NewImageConstraint(t.region, "http://ec2.us-east-1.amazonaws.com", "precise", t.arches, "")
		imageConstraint.Storage = t.storage
		images, err := GetImageIdMetadata([]string{s.baseURL}, DefaultIndexPath, &imageConstraint)
		if !c.Check(err, IsNil) {
			continue
		}
		c.Check(images, DeepEquals, t.images)
	}
}
