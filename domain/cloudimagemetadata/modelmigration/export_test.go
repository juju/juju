// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type exportSuite struct {
	coordinator *MockCoordinator
	service     *MockExportService
	clock       clock.Clock
}

func TestExportSuite(t *testing.T) {
	tc.Run(t, &exportSuite{})
}

func (s *exportSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockExportService(ctrl)
	s.clock = clock.WallClock

	return ctrl
}

func (s *exportSuite) newExportOperation(c *tc.C) *exportOperation {
	return &exportOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
		clock:   s.clock,
	}
}

// TestRegisterExport tests the registration of export operations with the coordinator.
func (s *exportSuite) TestRegisterExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterExport(s.coordinator, loggertesting.WrapCheckLog(c), clock.WallClock)
}

// TestExport verifies the export of cloud image metadata to the model. It creates some metadata with different values
// and check that all of them are added to the model.
func (s *exportSuite) TestExport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	creationTime := s.clock.Now()
	defaultAttr := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region",
		Version:         "version",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType",
		RootStorageSize: ptr(uint64(12)),
		Source:          "custom",
	}
	attr1, attr2, attr3 := defaultAttr, defaultAttr, defaultAttr
	attr2.Stream = "stream2"
	attr3.RootStorageSize = nil

	expected := []cloudimagemetadata.Metadata{{
		MetadataAttributes: attr1,
		Priority:           41,
		ImageID:            "attr1",
		CreationTime:       creationTime,
	}, {
		MetadataAttributes: attr2,
		Priority:           42,
		ImageID:            "attr2",
		CreationTime:       creationTime,
	}, {
		MetadataAttributes: attr3,
		Priority:           43,
		ImageID:            "attr3",
		CreationTime:       creationTime,
	}}
	dst := description.NewModel(description.ModelArgs{})
	s.service.EXPECT().AllCloudImageMetadata(gomock.Any()).Return(expected, nil)

	// Act
	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	actualCloudMetadata := dst.CloudImageMetadata()
	obtained := transformMetadataFromDescriptionToDomain(actualCloudMetadata)
	c.Assert(obtained, tc.DeepEquals, expected)
}

// TestExportFailGetAllImage verifies that the export operation handles failure when retrieving cloud image metadata,
// returning the underlying failure.
func (s *exportSuite) TestExportFailGetAllImage(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expected := errors.New("error")
	dst := description.NewModel(description.ModelArgs{})
	s.service.EXPECT().AllCloudImageMetadata(gomock.Any()).Return(nil, expected)

	// Act
	op := s.newExportOperation(c)
	err := op.Execute(c.Context(), dst)

	// Assert
	c.Assert(err, tc.ErrorIs, expected)
}

func ptr[T any](u T) *T {
	return &u
}
