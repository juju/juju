// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package imagemetadata_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	"github.com/juju/juju/environs/simplestreams"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/jujuclient/jujuclienttesting"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state/cloudimagemetadata"
	"github.com/juju/juju/testing"
)

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(c *gc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
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
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:")
	useTestImageData(c, testImagesData)
}

func (s *imageMetadataUpdateSuite) TearDownSuite(c *gc.C) {
	useTestImageData(c, nil)
	s.BaseSuite.TearDownSuite(c)
}

func (s *imageMetadataUpdateSuite) TestUpdateFromPublishedImagesForProviderWithNoRegions(c *gc.C) {
	// This will save all available image metadata.
	saved := []cloudimagemetadata.Metadata{}

	// testingEnvConfig prepares an environment configuration using
	// the dummy provider since it doesn't implement simplestreams.HasRegion.
	s.state.environConfig = func() (*config.Config, error) {
		cfg, err := config.New(config.NoDefaults, dummy.SampleConfig())
		c.Assert(err, jc.ErrorIsNil)
		env, err := environs.Prepare(
			modelcmd.BootstrapContext(testing.Context(c)), configstore.NewMem(),
			jujuclienttesting.NewMemStore(),
			"dummycontroller", environs.PrepareForBootstrapParams{Config: cfg},
		)
		c.Assert(err, jc.ErrorIsNil)
		return env.Config(), err
	}

	s.state.saveMetadata = func(m []cloudimagemetadata.Metadata) error {
		saved = append(saved, m...)
		return nil
	}

	err := s.api.UpdateFromPublishedImages()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, environConfig)
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

func (p mockEnvironProvider) PrepareForBootstrap(environs.BootstrapContext, environs.PrepareForBootstrapParams) (environs.Environ, error) {
	return &mockEnviron{}, nil
}

func (p mockEnvironProvider) Open(*config.Config) (environs.Environ, error) {
	return &mockEnviron{}, nil
}

var _ = gc.Suite(&regionMetadataSuite{})

type regionMetadataSuite struct {
	baseImageMetadataSuite

	env *mockEnviron

	saved    []cloudimagemetadata.Metadata
	expected []cloudimagemetadata.Metadata
}

func (s *regionMetadataSuite) SetUpSuite(c *gc.C) {
	s.baseImageMetadataSuite.SetUpSuite(c)

	s.env = &mockEnviron{}

	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	// Prepare mock http transport for overriding metadata and images output in tests.
	useTestImageData(c, testImagesData)
}

func (s *regionMetadataSuite) TearDownSuite(c *gc.C) {
	useTestImageData(c, nil)
	s.baseImageMetadataSuite.TearDownSuite(c)
}

func (s *regionMetadataSuite) SetUpTest(c *gc.C) {
	s.baseImageMetadataSuite.SetUpTest(c)

	s.saved = nil
	s.expected = nil
}

func (s *regionMetadataSuite) setExpectations(c *gc.C) {
	// This will only save image metadata specific to provider cloud spec.
	s.expected = []cloudimagemetadata.Metadata{
		cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				RootStorageType: "ebs",
				VirtType:        "pv",
				Arch:            "amd64",
				Series:          "trusty",
				Region:          "dummy_region",
				Source:          "default cloud images",
				Stream:          "released"},
			10,
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
			10,
			"ami-26745463",
		},
	}

	// testingEnvConfig prepares an environment configuration using
	// mock provider which impelements simplestreams.HasRegion interface.
	s.state.environConfig = func() (*config.Config, error) {
		return s.env.Config(), nil
	}

	s.state.saveMetadata = func(m []cloudimagemetadata.Metadata) error {
		s.saved = append(s.saved, m...)
		return nil
	}
}

func (s *regionMetadataSuite) checkStoredPublished(c *gc.C) {
	err := s.api.UpdateFromPublishedImages()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, environConfig, environConfig, saveMetadata)
	c.Assert(s.saved, jc.SameContents, s.expected)
}

func (s *regionMetadataSuite) TestUpdateFromPublishedImagesForProviderWithRegions(c *gc.C) {
	// This will only save image metadata specific to provider cloud spec.
	s.setExpectations(c)
	s.checkStoredPublished(c)
}

const (
	indexContent = `{
    "index": {
        "com.ubuntu.cloud:%v": {
            "updated": "Fri, 17 Jul 2015 13:42:48 +1000",
            "format": "products:1.0",
            "datatype": "image-ids",
            "cloudname": "custom",
            "clouds": [
                {
                    "region": "%v",
                    "endpoint": "%v"
                }
            ],
            "path": "streams/v1/products.json",
            "products": [
                "com.ubuntu.cloud:server:14.04:%v"
            ]
        }
    },
    "updated": "Fri, 17 Jul 2015 13:42:48 +1000",
    "format": "index:1.0"
}`

	productContent = `{
    "products": {
        "com.ubuntu.cloud:server:14.04:%v": {
            "version": "14.04",
            "arch": "%v",
            "versions": {
                "20151707": {
                    "items": {
                        "%v": {
                            "id": "%v",
                            "root_store": "%v",
                            "virt": "%v",
                            "region": "%v",
                            "endpoint": "%v"
                        }
                    }
                }
            }
        }
     },
    "updated": "Fri, 17 Jul 2015 13:42:48 +1000",
    "format": "products:1.0",
    "content_id": "com.ubuntu.cloud:custom"
}`
)

func writeTempFiles(c *gc.C, metadataDir string, expected []struct{ path, content string }) {
	for _, pair := range expected {
		path := filepath.Join(metadataDir, pair.path)
		err := os.MkdirAll(filepath.Dir(path), 0755)
		c.Assert(err, jc.ErrorIsNil)
		err = ioutil.WriteFile(path, []byte(pair.content), 0644)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *regionMetadataSuite) createTestDataSource(c *gc.C, dsID string, files []struct{ path, content string }) int {
	metadataDir := c.MkDir()
	writeTempFiles(c, metadataDir, files)

	ds := simplestreams.NewURLDataSource(dsID, "file://"+metadataDir, false, 20, false)
	environs.RegisterImageDataSourceFunc(dsID, func(environs.Environ) (simplestreams.DataSource, error) {
		return ds, nil
	})
	s.AddCleanup(func(*gc.C) {
		environs.UnregisterImageDataSourceFunc(dsID)
	})
	return ds.Priority()
}

func (s *regionMetadataSuite) setupMetadata(c *gc.C, dsID string, cloudSpec simplestreams.CloudSpec, metadata cloudimagemetadata.Metadata) int {
	files := []struct{ path, content string }{{
		path:    "streams/v1/index.json",
		content: fmt.Sprintf(indexContent, metadata.Source, metadata.Region, cloudSpec.Endpoint, metadata.Arch),
	}, {
		path:    "streams/v1/products.json",
		content: fmt.Sprintf(productContent, metadata.Arch, metadata.Arch, metadata.ImageId, metadata.ImageId, metadata.RootStorageType, metadata.VirtType, metadata.Region, cloudSpec.Endpoint),
	}, {
		path:    "wayward/file.txt",
		content: "ghi",
	}}
	return s.createTestDataSource(c, dsID, files)
}

func (s *regionMetadataSuite) TestUpdateFromPublishedImagesMultipleDS(c *gc.C) {
	s.setExpectations(c)

	// register another data source
	cloudSpec, err := s.env.Region()
	c.Assert(err, jc.ErrorIsNil)
	anotherDS := "second ds"

	m1 := s.expected[0]
	priority := s.setupMetadata(c, anotherDS, cloudSpec, m1)
	m1.Source = anotherDS
	m1.Priority = priority

	s.expected = append(s.expected, m1)

	err = s.api.UpdateFromPublishedImages()
	c.Assert(err, jc.ErrorIsNil)
	s.assertCalls(c, environConfig, environConfig, saveMetadata, environConfig, saveMetadata)
	c.Assert(s.saved, jc.SameContents, s.expected)
}

func (s *regionMetadataSuite) TestUpdateFromPublishedImagesMultipleDSError(c *gc.C) {
	s.setExpectations(c)

	// register another data source that would error
	files := []struct{ path, content string }{{
		path:    "wayward/file.txt",
		content: "ghi",
	}}
	s.createTestDataSource(c, "error in ds", files)

	s.checkStoredPublished(c)
}
