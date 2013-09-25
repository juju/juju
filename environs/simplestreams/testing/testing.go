// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

// Provides a TestDataSuite which creates and provides http access to sample simplestreams metadata.

import (
	"fmt"
	"net/http"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/environs/jujutest"
	"launchpad.net/juju-core/environs/simplestreams"
	"launchpad.net/juju-core/testing/testbase"
)

var imageData = map[string]string{
	"/streams/v1/index.json": `
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
			 "endpoint": "https://ec2.us-east-1.amazonaws.com"
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
		   "datatype": "content-download",
		   "path": "streams/v1/com.ubuntu.cloud:released:download.json",
		   "updated": "Wed, 01 May 2013 13:30:37 +0000",
		   "products": [
			"com.ubuntu.cloud:server:12.10:amd64",
			"com.ubuntu.cloud:server:13.04:amd64"
		   ],
		   "format": "products:1.0"
		  },
		  "com.ubuntu.juju:released:tools": {
		   "updated": "Mon, 05 Aug 2013 11:07:04 +0000",
		   "datatype": "content-download",
		   "format": "products:1.0",
		   "products": [
		     "com.ubuntu.juju:12.04:amd64",
		     "com.ubuntu.juju:12.04:arm",
		     "com.ubuntu.juju:13.04:amd64",
		     "com.ubuntu.juju:13.04:arm"
		   ],
		   "path": "streams/v1/tools_metadata.json"
		  }
		 },
         "mirrors": {
          "com.ubuntu.juju:released:tools": [
             {
              "datatype": "content-download",
              "path": "streams/v1/tools_metadata:public-mirrors.json",
		      "clouds": [
			   {
			    "region": "us-east-1",
			    "endpoint": "https://ec2.us-east-1.amazonaws.com"
			   }
		      ],
              "updated": "Wed, 14 Aug 2013 13:46:17 +0000",
              "format": "mirrors:1.0"
             },
             {
              "datatype": "content-download",
              "path": "streams/v1/tools_metadata:more-mirrors.json",
              "updated": "Wed, 14 Aug 2013 13:46:17 +0000",
              "format": "mirrors:1.0"
             }
		  ]
		 },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/streams/v1/tools_metadata.json": `
{
 "content_id": "com.ubuntu.juju:tools",
 "datatype": "content-download",
 "updated": "Tue, 04 Jun 2013 13:50:31 +0000",
 "format": "products:1.0",
 "products": {
  "com.ubuntu.juju:12.04:amd64": {
   "arch": "amd64",
   "release": "precise",
   "versions": {
    "20130806": {
     "items": {
      "1130preciseamd64": {
       "version": "1.13.0",
       "size": 2973595,
       "path": "tools/releases/20130806/juju-1.13.0-precise-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "447aeb6a934a5eaec4f703eda4ef2dde"
      }
     }
    }
   }
  },
  "com.ubuntu.juju:13.04:amd64": {
   "arch": "amd64",
   "release": "raring",
   "versions": {
    "20130806": {
     "items": {
      "1130raringamd64": {
       "version": "1.13.0",
       "size": 2973173,
       "path": "tools/releases/20130806/juju-1.13.0-raring-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      },
      "1140raringamd64": {
       "version": "1.14.0",
       "size": 2973173,
       "path": "tools/releases/20130806/juju-1.14.0-raring-amd64.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      }
     }
    }
   }
  },
  "com.ubuntu.juju:12.04:arm": {
   "arch": "arm",
   "release": "precise",
   "versions": {
    "20130806": {
     "items": {
      "201precisearm": {
       "version": "2.0.1",
       "size": 1951096,
       "path": "tools/releases/20130806/juju-2.0.1-precise-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "f65a92b3b41311bdf398663ee1c5cd0c"
      },
      "1114precisearm": {
       "version": "1.11.4",
       "size": 1951096,
       "path": "tools/releases/20130806/juju-1.11.4-precise-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "f65a92b3b41311bdf398663ee1c5cd0c"
      }
     }
    },
    "20130803": {
     "items": {
      "1114precisearm": {
       "version": "1.11.4",
       "size": 2851541,
       "path": "tools/releases/20130803/juju-1.11.4-precise-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      },
      "1115precisearm": {
       "version": "1.11.5",
       "size": 2031281,
       "path": "tools/releases/20130803/juju-1.11.5-precise-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "df07ac5e1fb4232d4e9aa2effa57918a"
      }
     }
    }
   }
  },
  "com.ubuntu.juju:13.04:arm": {
   "arch": "arm",
   "release": "raring",
   "versions": {
    "20130806": {
     "items": {
      "1114raringarm": {
       "version": "1.11.4",
       "size": 1950327,
       "path": "tools/releases/20130806/juju-1.11.4-raring-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "6472014e3255e3fe7fbd3550ef3f0a11"
      },
      "201raringarm": {
       "version": "2.0.1",
       "size": 1950327,
       "path": "tools/releases/20130806/juju-2.0.1-raring-arm.tgz",
       "ftype": "tar.gz",
       "sha256": "6472014e3255e3fe7fbd3550ef3f0a11"
      }
     }
    }
   }
  }
 }
}
`,
	"/streams/v1/tools_metadata:public-mirrors.json": `
{
  "mirrors": {
    "com.ubuntu.juju:released:tools": [
      {
        "mirror": "http://some-mirror/",
        "path": "com.ubuntu.juju:download.json",
        "format": "products:1.0",
        "clouds": [
          {
            "endpoint": "https://ec2.us-east-1.amazonaws.com",
            "region": "us-east-1"
          }
        ]
      },
      {
        "mirror": "http://another-mirror/",
        "path": "com.ubuntu.juju:download.json",
        "format": "products:1.0",
        "clouds": [
          {
            "endpoint": "https://ec2.us-west-1.amazonaws.com",
            "region": "us-west-1"
          }
        ]
      }
    ]
  },
  "format": "mirrors:1.0",
  "updated": "Mon, 05 Aug 2013 11:07:04 +0000"
}
`,
	"/streams/v1/tools_metadata:more-mirrors.json": `
{
  "mirrors": {
    "com.ubuntu.juju:released:tools": [
      {
        "mirror": "http://big-mirror/",
        "path": "big:download.json",
        "clouds": [
          {
            "endpoint": "https://some-endpoint.com",
            "region": "some-region"
          }
        ]
      }
    ]
  },
  "format": "mirrors:1.0",
  "updated": "Mon, 05 Aug 2013 11:07:04 +0000"
}
`,
	"/streams/v1/image_metadata.json": `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "products": {
  "com.ubuntu.cloud:server:12.04:amd64": {
   "release": "precise",
   "version": "12.04",
   "arch": "amd64",
   "region": "au-east-1",
   "endpoint": "https://somewhere",
   "versions": {
    "20121218": {
     "region": "au-east-2",
     "endpoint": "https://somewhere-else",
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
       "endpoint": "https://ec2.us-east-1.amazonaws.com"
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
       "endpoint": "https://ec2.us-east-1.amazonaws.com"
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
   "endpoint": "https://ec2.us-east-1.amazonaws.com",
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
    "endpoint": "https://ec2.us-west-3.amazonaws.com"
   }
  }
 },
 "format": "products:1.0"
}
`,
}

var testRoundTripper *jujutest.ProxyRoundTripper

func init() {
	testRoundTripper = &jujutest.ProxyRoundTripper{}
	simplestreams.RegisterProtocol("test", testRoundTripper)
}

type TestDataSuite struct{}

func (s *TestDataSuite) SetUpSuite(c *gc.C) {
	testRoundTripper.Sub = jujutest.NewCannedRoundTripper(
		imageData, map[string]int{"test://unauth": http.StatusUnauthorized})
}

func (s *TestDataSuite) TearDownSuite(c *gc.C) {
	testRoundTripper.Sub = nil
}

func AssertExpectedSources(c *gc.C, obtained []simplestreams.DataSource, baseURLs []string) {
	var obtainedURLs = make([]string, len(baseURLs))
	for i, source := range obtained {
		url, err := source.URL("")
		c.Assert(err, gc.IsNil)
		obtainedURLs[i] = url
	}
	c.Assert(obtainedURLs, gc.DeepEquals, baseURLs)
}

type LocalLiveSimplestreamsSuite struct {
	testbase.LoggingSuite
	Source          simplestreams.DataSource
	RequireSigned   bool
	DataType        string
	ValidConstraint simplestreams.LookupConstraint
}

func (s *LocalLiveSimplestreamsSuite) SetUpSuite(c *gc.C) {
	s.LoggingSuite.SetUpSuite(c)
}

func (s *LocalLiveSimplestreamsSuite) TearDownSuite(c *gc.C) {
	s.LoggingSuite.TearDownSuite(c)
}

const (
	Index_v1   = "index:1.0"
	Product_v1 = "products:1.0"
	Mirror_v1  = "mirrors:1.0"
)

type testConstraint struct {
	simplestreams.LookupParams
}

func NewTestConstraint(params simplestreams.LookupParams) *testConstraint {
	return &testConstraint{LookupParams: params}
}

func (tc *testConstraint) Ids() ([]string, error) {
	version, err := simplestreams.SeriesVersion(tc.Series[0])
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(tc.Arches))
	for i, arch := range tc.Arches {
		ids[i] = fmt.Sprintf("com.ubuntu.cloud:server:%s:%s", version, arch)
	}
	return ids, nil
}

func init() {
	// Ensure out test struct can have its tags extracted.
	simplestreams.RegisterStructTags(TestItem{})
}

type TestItem struct {
	Id          string `json:"id"`
	Storage     string `json:"root_store"`
	VType       string `json:"virt"`
	Arch        string `json:"arch"`
	RegionAlias string `json:"crsn"`
	RegionName  string `json:"region"`
	Endpoint    string `json:"endpoint"`
}

func (s *LocalLiveSimplestreamsSuite) IndexPath() string {
	if s.RequireSigned {
		return simplestreams.DefaultIndexPath + ".sjson"
	}
	return simplestreams.UnsignedIndex
}

func (s *LocalLiveSimplestreamsSuite) TestGetIndex(c *gc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, gc.IsNil)
	c.Assert(indexRef.Format, gc.Equals, Index_v1)
	c.Assert(indexRef.Source, gc.Equals, s.Source)
	c.Assert(len(indexRef.Indexes) > 0, gc.Equals, true)
}

func (s *LocalLiveSimplestreamsSuite) GetIndexRef(format string) (*simplestreams.IndexReference, error) {
	params := simplestreams.ValueParams{
		DataType:      s.DataType,
		ValueTemplate: TestItem{},
	}
	return simplestreams.GetIndexWithFormat(s.Source, s.IndexPath(), format, s.RequireSigned, params)
}

func (s *LocalLiveSimplestreamsSuite) TestGetIndexWrongFormat(c *gc.C) {
	_, err := s.GetIndexRef("bad")
	c.Assert(err, gc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathExists(c *gc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, gc.IsNil)
	path, err := indexRef.GetProductsPath(s.ValidConstraint)
	c.Assert(err, gc.IsNil)
	c.Assert(path, gc.Not(gc.Equals), "")
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathInvalidCloudSpec(c *gc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, gc.IsNil)
	ic := NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: simplestreams.CloudSpec{"bad", "spec"},
		Series:    []string{"precise"},
	})
	_, err = indexRef.GetProductsPath(ic)
	c.Assert(err, gc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) TestGetProductsPathInvalidProductSpec(c *gc.C) {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, gc.IsNil)
	ic := NewTestConstraint(simplestreams.LookupParams{
		CloudSpec: s.ValidConstraint.Params().CloudSpec,
		Series:    []string{"precise"},
		Arches:    []string{"bad"},
		Stream:    "spec",
	})
	_, err = indexRef.GetProductsPath(ic)
	c.Assert(err, gc.NotNil)
}

func (s *LocalLiveSimplestreamsSuite) AssertGetMetadata(c *gc.C) *simplestreams.CloudMetadata {
	indexRef, err := s.GetIndexRef(Index_v1)
	c.Assert(err, gc.IsNil)
	metadata, err := indexRef.GetCloudMetadataWithFormat(s.ValidConstraint, Product_v1, s.RequireSigned)
	c.Assert(err, gc.IsNil)
	c.Assert(metadata.Format, gc.Equals, Product_v1)
	c.Assert(len(metadata.Products) > 0, gc.Equals, true)
	return metadata
}

func (s *LocalLiveSimplestreamsSuite) TestGetCloudMetadataWithFormat(c *gc.C) {
	s.AssertGetMetadata(c)
}

func (s *LocalLiveSimplestreamsSuite) AssertGetItemCollections(c *gc.C, version string) *simplestreams.ItemCollection {
	metadata := s.AssertGetMetadata(c)
	metadataCatalog := metadata.Products["com.ubuntu.cloud:server:12.04:amd64"]
	ic := metadataCatalog.Items[version]
	return ic
}
