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
	"github.com/juju/juju/environs/simplestreams"
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
		  "com.ubuntu.cloud:released:aws": {
		   "updated": "Wed, 01 May 2013 13:31:26 +0000",
		   "clouds": [
			{
			 "region": "dummy_region",
			 "endpoint": "https://anywhere"
			},
			{
			 "region": "another_dummy_region",
			 "endpoint": ""
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
		 "updated": "Wed, 27 May 2015 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/streams/v1/image_metadata.json": `
{
 "updated": "Wed, 27 May 2015 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:released:aws",
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
       "crsn": "da1",
       "id": "ami-36745463"
      },
      "nzww1pe2": {
       "root_store": "ebs",
       "virt": "pv",
       "crsn": "da2",
       "id": "ami-1136745463"
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
   "versions": {
    "20121218": {
     "items": {
      "usww1pe": {
       "root_store": "ebs",
       "virt": "pv",
       "crsn": "da1",
       "id": "ami-26745463"
      },
      "usww1pe2": {
       "root_store": "ebs",
       "virt": "pv",
       "crsn": "da2",
       "id": "ami-1126745463"
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
   "da1": {
    "region": "dummy_region",
    "endpoint": "https://anywhere"
   },
   "da2": {
    "region": "another_dummy_region",
    "endpoint": ""
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

func (s *imageMetadataUpdateSuite) TestUpdateFromPublishedImagesForProviderWithNoRegions(c *gc.C) {
	// This will save all available image metadata.
	saved := []cloudimagemetadata.Metadata{}

	// testingEnvConfig prepares an environment configuration using
	// the dummy provider since it doesn't implement simplestreams.HasRegion.
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
	c.Assert(err, gc.ErrorMatches, ".*environment cloud specification cannot be determined.*")
	s.assertCalls(c, []string{environConfig})

	c.Assert(saved, jc.SameContents, []cloudimagemetadata.Metadata{})
}

// mockConfig returns a configuration for the usage of the
// mock provider below.
func mockConfig() testing.Attrs {
	return dummy.SampleConfig().Merge(testing.Attrs{
		"type": "mock",
	})
}

// mockEnviron is an environment without networking support.
type mockEnviron struct {
	environs.Environ
}

func (e mockEnviron) Config() *config.Config {
	cfg, err := config.New(config.NoDefaults, mockConfig())
	if err != nil {
		panic("invalid configuration for testing")
	}
	return cfg
}

// Region is specified in the HasRegion interface.
func (e *mockEnviron) Region() (simplestreams.CloudSpec, error) {
	return simplestreams.CloudSpec{
		Region:   "dummy_region",
		Endpoint: "https://anywhere",
	}, nil
}

// mockEnvironProvider is the smallest possible provider to
// test image metadata retrieval with region support.
type mockEnvironProvider struct {
	environs.EnvironProvider
}

func (p mockEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	return &mockEnviron{}, nil
}

func (p mockEnvironProvider) Open(*config.Config) (environs.Environ, error) {
	return &mockEnviron{}, nil
}

var _ = gc.Suite(&regionMetadataSuite{})

type regionMetadataSuite struct {
	baseImageMetadataSuite

	provider mockEnvironProvider
}

func (s *regionMetadataSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)

	s.provider = mockEnvironProvider{}
	environs.RegisterProvider("mock", s.provider)
	useTestImageData(testImagesData)
}

func (s *regionMetadataSuite) TearDownSuite(c *gc.C) {
	useTestImageData(nil)
	s.BaseSuite.TearDownSuite(c)
}

func (s *regionMetadataSuite) SetUpTest(c *gc.C) {
	s.baseImageMetadataSuite.SetUpTest(c)
}

func (s *regionMetadataSuite) TestUpdateFromPublishedImagesForProviderWithRegions(c *gc.C) {
	// This will only save image metadata specific to provider cloud spec.
	saved := []cloudimagemetadata.Metadata{}
	expected := []cloudimagemetadata.Metadata{
		cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				RootStorageType: "ebs",
				VirtType:        "pv",
				Arch:            "amd64",
				Series:          "trusty",
				Region:          "dummy_region",
				Source:          "default cloud images",
				Stream:          "released"},
			"ami-36745463",
		},
		cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				RootStorageType: "ebs",
				VirtType:        "pv",
				Arch:            "amd64",
				Series:          "precise",
				Region:          "dummy_region",
				Source:          "default cloud images",
				Stream:          "released"},
			"ami-26745463",
		},
	}

	// testingEnvConfig prepares an environment configuration using
	// mock provider which impelements simplestreams.HasRegion interface.
	s.state.environConfig = func() (*config.Config, error) {
		s.calls = append(s.calls, environConfig)
		env := &mockEnviron{}
		return env.Config(), nil
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
