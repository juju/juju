// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/agent/provisioner"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/environs/imagemetadata"
	imagetesting "github.com/juju/juju/environs/imagemetadata/testing"
	sstesting "github.com/juju/juju/environs/simplestreams/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/juju/keys"
	"github.com/juju/juju/rpc/params"
)

// useTestImageData causes the given content to be served when published metadata is requested.
func useTestImageData(c *tc.C, files map[string]string) {
	if files != nil {
		sstesting.SetRoundTripperFiles(sstesting.AddSignedFiles(c, files), nil)
	} else {
		sstesting.SetRoundTripperFiles(nil, nil)
	}
}

type ImageMetadataSuite struct {
	provisionerSuite
}

var _ = tc.Suite(&ImageMetadataSuite{})

func (s *ImageMetadataSuite) SetUpSuite(c *tc.C) {
	s.provisionerSuite.SetUpSuite(c)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	s.AddCleanup(func(c *tc.C) {
		server.Close()
	})

	// Make sure that there is nothing in data sources.
	// Each individual tests will decide if it needs metadata there.
	imagetesting.PatchOfficialDataSources(&s.CleanupSuite, server.URL)
	s.PatchValue(&imagemetadata.SimplestreamsImagesPublicKey, sstesting.SignedMetadataPublicKey)
	s.PatchValue(&keys.JujuPublicKey, sstesting.SignedMetadataPublicKey)
	useTestImageData(c, nil)
}

func (s *ImageMetadataSuite) TearDownSuite(c *tc.C) {
	useTestImageData(c, nil)
	s.provisionerSuite.TearDownSuite(c)
}

func (s *ImageMetadataSuite) SetUpTest(c *tc.C) {
	s.provisionerSuite.SetUpTest(c)
}

func (s *ImageMetadataSuite) TestMetadataNone(c *tc.C) {
	api, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           s.authorizer,
		State_:          s.ControllerModel(c).State(),
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: s.ControllerDomainServices(c),
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	result, err := api.ProvisioningInfo(context.Background(), s.getTestMachinesTags(c))
	c.Assert(err, tc.ErrorIsNil)

	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i := range result.Results {
		expected[i] = nil
	}
	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) TestMetadataFromState(c *tc.C) {
	st := s.ControllerModel(c).State()
	domainServices := s.ControllerDomainServices(c)
	metadataService := domainServices.CloudImageMetadata()
	api, err := provisioner.MakeProvisionerAPI(context.Background(), facadetest.ModelContext{
		Auth_:           s.authorizer,
		State_:          st,
		StatePool_:      s.StatePool(),
		Resources_:      s.resources,
		DomainServices_: domainServices,
		Logger_:         loggertesting.WrapCheckLog(c),
	})
	c.Assert(err, tc.ErrorIsNil)

	expected := s.expectedDataSourceImageMetadata()

	// Write metadata to state.
	metadata := s.convertCloudImageMetadata(expected[0])
	for _, m := range metadata {
		err := metadataService.SaveMetadata(context.Background(), []cloudimagemetadata.Metadata{m})
		c.Assert(err, tc.ErrorIsNil)
	}

	result, err := api.ProvisioningInfo(context.Background(), s.getTestMachinesTags(c))
	c.Assert(err, tc.ErrorIsNil)

	s.assertImageMetadataResults(c, result, expected...)
}

func (s *ImageMetadataSuite) getTestMachinesTags(c *tc.C) params.Entities {

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
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Region:          one.Region,
				Version:         one.Version,
				Arch:            one.Arch,
				VirtType:        one.VirtType,
				RootStorageType: one.RootStorageType,
				Source:          one.Source,
				Stream:          one.Stream,
			},
			Priority:     one.Priority,
			ImageID:      one.ImageId,
			CreationTime: time.Now(),
		}
	}
	return expected
}

func (s *ImageMetadataSuite) expectedDataSourceImageMetadata() [][]params.CloudImageMetadata {
	expected := make([][]params.CloudImageMetadata, len(s.machines))
	for i := range s.machines {
		expected[i] = []params.CloudImageMetadata{
			{ImageId: "ami-26745463",
				Region:          "dummy_region",
				Version:         "12.10",
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
	c *tc.C, obtained params.ProvisioningInfoResults, expected ...[]params.CloudImageMetadata,
) {
	c.Assert(obtained.Results, tc.HasLen, len(expected))
	for i, one := range obtained.Results {
		c.Assert(one.Error, tc.IsNil)
		// We are only concerned with images here
		c.Assert(one.Result.ImageMetadata, tc.DeepEquals, expected[i])
	}
}
