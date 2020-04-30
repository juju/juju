// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	"github.com/juju/juju/juju/keys"
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
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
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
	for i := range result.Results {
		expected[i] = nil
	}
	s.assertImageMetadataResults(c, result, expected...)
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
			0,
		}
	}
	return expected
}

func (s *ImageMetadataSuite) expectedDataSoureImageMetadata() [][]params.CloudImageMetadata {
	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i := range s.machines {
		expected[i] = []params.CloudImageMetadata{
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

func (s *ImageMetadataSuite) assertImageMetadataResults(
	c *gc.C, obtained params.ProvisioningInfoResultsV10, expected ...[]params.CloudImageMetadata,
) {
	c.Assert(obtained.Results, gc.HasLen, len(expected))
	for i, one := range obtained.Results {
		// We are only concerned with images here
		c.Assert(one.Result.ImageMetadata, gc.DeepEquals, expected[i])
	}
}
