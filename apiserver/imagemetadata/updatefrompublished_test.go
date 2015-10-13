// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/jujutest"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

var (
	testRoundTripper = &jujutest.ProxyRoundTripper{}
)

func init() {
	// Prepare mock http transport for overriding metadata and images output in tests.
	testRoundTripper.RegisterForScheme("test")
}

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(files map[string]string) {
	if files != nil {
		testRoundTripper.Sub = jujutest.NewCannedRoundTripper(files, nil)
		imagemetadata.DefaultBaseURL = "test:"
	} else {
		testRoundTripper.Sub = nil
		imagemetadata.DefaultBaseURL = ""
	}
}

// TODO (anastasiamac 2015-09-04) This metadata is so verbose.
// Need to generate the text by creating a struct and marshalling it.
var testImagesData = map[string]string{
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
			"com.ubuntu.cloud:server:14.04:amd64"
		   ],
		   "path": "streams/v1/image_metadata.json"
		   }
		  },
		 "updated": "Wed, 01 May 2013 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/streams/v1/image_metadata.json": `
{
 "updated": "Wed, 01 May 2013 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
 "region": "nz-east-1",
 "endpoint": "https://anywhere",
 "products": {
  "com.ubuntu.cloud:server:14.04:amd64": {
   "release": "trusty",
   "version": "14.04",
   "arch": "amd64",
   "versions": {
    "20140118": {
     "items": {
      "nzww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "id": "ami-36745463"
      }
     },
     "pubname": "ubuntu-trusty-14.04-amd64-server-20140118",
     "label": "release"
    }
   }
  },
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
      }
     },
     "pubname": "ubuntu-precise-12.04-amd64-server-20121218",
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

var _ = gc.Suite(&imageMetadataUpdateSuite{})

type imageMetadataUpdateSuite struct {
	baseImageMetadataSuite
}

func (s *imageMetadataUpdateSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	useTestImageData(testImagesData)
}

func (s *imageMetadataUpdateSuite) TearDownSuite(c *gc.C) {
	useTestImageData(nil)
	s.BaseSuite.TearDownSuite(c)
}

func (s *imageMetadataUpdateSuite) SetUpTest(c *gc.C) {
	s.baseImageMetadataSuite.SetUpTest(c)
}

func (s *imageMetadataUpdateSuite) TestUpdateFromPublishedImages(c *gc.C) {
	saved := []cloudimagemetadata.Metadata{}
	expected := []cloudimagemetadata.Metadata{
		cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				RootStorageType: "ebs",
				VirtType:        "pv",
				Arch:            "amd64",
				Series:          "trusty",
				Region:          "nz-east-1",
				Source:          "public",
				Stream:          "released"},
			"ami-36745463",
		},
		cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				RootStorageType: "ebs",
				VirtType:        "pv",
				Arch:            "amd64",
				Series:          "precise",
				Region:          "au-east-2",
				Source:          "public",
				Stream:          "released"},
			"ami-26745463",
		},
	}

	// testingEnvConfig prepares an environment configuration using
	// the dummy provider.
	s.state.environConfig = func() (*config.Config, error) {
		s.calls = append(s.calls, environConfig)
		cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
		c.Assert(err, jc.ErrorIsNil)
		env, err := environs.Prepare(cfg, envcmd.BootstrapContext(testing.Context(c)), configstore.NewMem())
		c.Assert(err, jc.ErrorIsNil)
		return env.Config(), err
	}

	s.state.saveMetadata = func(m cloudimagemetadata.Metadata) error {
		s.calls = append(s.calls, saveMetadata)
		saved = append(saved, m)
		return nil
	}

	err := s.api.UpdateFromPublishedImages()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, []string{environConfig, saveMetadata, saveMetadata})

	c.Assert(saved, jc.SameContents, expected)
}
