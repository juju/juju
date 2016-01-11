// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/provisioner"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/state/cloudimagemetadata"
)

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(c *gc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
	}
}

type ImageMetadataSuite struct {
	provisionerSuite
}

var _ = gc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *gc.C) {
	s.provisionerSuite.SetUpSuite(c)

	// Make sure that there is nothing in data sources.
	// Each individual tests will decide if it needs metadata there.
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, "test:/daily")
	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	useTestImageData(c, nil)
}

func (s *ImageMetadataSuite) TearDownSuite(c *gc.C) {
	useTestImageData(c, nil)
	s.provisionerSuite.TearDownSuite(c)
}

func (s *ImageMetadataSuite) SetUpTest(c *gc.C) {
	s.provisionerSuite.SetUpTest(c)
}

func (s *ImageMetadataSuite) TestMetadataNone(c *gc.C) {
	api, err := provisioner.NewProvisionerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.ProvisioningInfo(s.getTestMachinesTags(c))
	c.Assert(err, jc.ErrorIsNil)

	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i, _ := range result.Results {
		expected[i] = nil
	}
	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) TestMetadataNotInStateButInDataSources(c *gc.C) {
	// ensure metadata in data sources and not in state
	useTestImageData(c, testImagesData)

	criteria := cloudimagemetadata.MetadataFilter{Stream: "daily"}
	found, err := s.State.CloudImageMetadataStorage.FindMetadata(criteria)
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
	c.Assert(found, gc.HasLen, 0)

	api, err := provisioner.NewProvisionerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	result, err := api.ProvisioningInfo(s.getTestMachinesTags(c))
	c.Assert(err, jc.ErrorIsNil)

	expected := s.expectedDataSoureImageMetadata()
	s.assertImageMetadataResults(c, result, expected...)

	// Also make sure that these images metadata has been written to state for re-use
	saved, err := s.State.CloudImageMetadataStorage.FindMetadata(criteria)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(saved, gc.DeepEquals, map[string][]cloudimagemetadata.Metadata{
		"default cloud images": s.convertCloudImageMetadata(expected[0]),
	})
}

func (s *ImageMetadataSuite) TestMetadataFromState(c *gc.C) {
	api, err := provisioner.NewProvisionerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, jc.ErrorIsNil)

	expected := s.expectedDataSoureImageMetadata()

	// Write metadata to state.
	metadata := s.convertCloudImageMetadata(expected[0])
	for _, m := range metadata {
		err := s.State.CloudImageMetadataStorage.SaveMetadata(
			[]cloudimagemetadata.Metadata{m},
		)
		c.Assert(err, jc.ErrorIsNil)
	}

	result, err := api.ProvisioningInfo(s.getTestMachinesTags(c))
	c.Assert(err, jc.ErrorIsNil)

	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) getTestMachinesTags(c *gc.C) params.Entities {

	testMachines := make([]params.Entity, len(s.machines))

	for i, m := range s.machines {
		testMachines[i] = params.Entity{Tag: m.Tag().String()}
	}
	return params.Entities{Entities: testMachines}
}

func (s *ImageMetadataSuite) convertCloudImageMetadata(all []params.CloudImageMetadata) []cloudimagemetadata.Metadata {
	expected := make([]cloudimagemetadata.Metadata, len(all))
	for i, one := range all {
		expected[i] = cloudimagemetadata.Metadata{
			cloudimagemetadata.MetadataAttributes{
				Region:          one.Region,
				Version:         one.Version,
				Series:          one.Series,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.RootStorageType,
				Source:          one.Source,
				Stream:          one.Stream,
			},
			one.Priority,
			one.ImageId,
		}
	}
	return expected
}

func (s *ImageMetadataSuite) expectedDataSoureImageMetadata() [][]params.CloudImageMetadata {
	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i, _ := range s.machines {
		expected[i] = []params.CloudImageMetadata{
			{ImageId: "ami-1126745463",
				Region:          "another_dummy_region",
				Version:         "12.10",
				Series:          "quantal",
				Arch:            "amd64",
				VirtType:        "pv",
				RootStorageType: "ebs",
				Source:          "default cloud images",
				Stream:          "daily",
				Priority:        10,
			},
			{ImageId: "ami-26745463",
				Region:          "dummy_region",
				Version:         "12.10",
				Series:          "quantal",
				Arch:            "amd64",
				VirtType:        "pv",
				RootStorageType: "ebs",
				Stream:          "daily",
				Source:          "default cloud images",
				Priority:        10},
		}
	}
	return expected
}

func (s *ImageMetadataSuite) assertImageMetadataResults(c *gc.C, obtained params.ProvisioningInfoResults, expected ...[]params.CloudImageMetadata) {
	c.Assert(obtained.Results, gc.HasLen, len(expected))
	for i, one := range obtained.Results {
		// We are only concerned with images here
		c.Assert(one.Result.ImageMetadata, gc.DeepEquals, expected[i])
	}
}

// TODO (anastasiamac 2015-09-04) This metadata is so verbose.
// Need to generate the text by creating a struct and marshalling it.
var testImagesData = map[string]string{
	"/daily/streams/v1/index.json": `
		{
		 "index": {
		  "com.ubuntu.cloud:daily:aws": {
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
			"com.ubuntu.cloud.daily:server:12.10:amd64",
			"com.ubuntu.cloud.daily:server:14.04:amd64"
		   ],
		   "path": "streams/v1/image_metadata.json"
		   }
		  },
		 "updated": "Wed, 27 May 2015 13:31:26 +0000",
		 "format": "index:1.0"
		}
`,
	"/daily/streams/v1/image_metadata.json": `
{
 "updated": "Wed, 27 May 2015 13:31:26 +0000",
 "content_id": "com.ubuntu.cloud:daily:aws",
 "products": {
  "com.ubuntu.cloud.daily:server:14.04:amd64": {
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
  "com.ubuntu.cloud.daily:server:12.10:amd64": {
   "release": "quantal",
   "version": "12.10",
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
     "pubname": "ubuntu-quantal-12.10-amd64-server-20121218",
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
