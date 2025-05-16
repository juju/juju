// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"fmt"
	stdtesting "testing"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v9"
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/internal/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type importSuite struct {
	coordinator *MockCoordinator
	service     *MockImportService
	clock       clock.Clock
}

func TestImportSuite(t *stdtesting.T) { tc.Run(t, &importSuite{}) }
func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.coordinator = NewMockCoordinator(ctrl)
	s.service = NewMockImportService(ctrl)
	s.clock = clock.WallClock

	return ctrl
}

func (s *importSuite) newImportOperation(c *tc.C) *importOperation {
	return &importOperation{
		service: s.service,
		logger:  loggertesting.WrapCheckLog(c),
		clock:   s.clock,
	}
}

// TestRegisterImport  tests the registration of import operations with the coordinator.
func (s *importSuite) TestRegisterImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.coordinator.EXPECT().Add(gomock.Any())

	RegisterImport(s.coordinator, loggertesting.WrapCheckLog(c), clock.WallClock)
}

// TestImport tests the import operation by verifying the SaveMetadata call with transformed metadata against the service.
// It creates several different metadata into the model and check that SaveMetadata is called with the right arguments
func (s *importSuite) TestImport(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	creationTime := time.Now()
	defaultArgs := description.CloudImageMetadataArgs{
		Stream:          "stream",
		Region:          "region",
		Version:         "version",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType",
		RootStorageSize: ptr(uint64(128)),
		DateCreated:     creationTime.UnixNano(),
		Source:          "source",
		Priority:        40,
		ImageId:         "attr0",
	}
	args := []description.CloudImageMetadataArgs{
		suffix(defaultArgs, 0, creationTime, customSource),
		suffix(defaultArgs, 1, creationTime, customSource),
		suffix(defaultArgs, 2, creationTime, customSource)}
	dst := description.NewModel(description.ModelArgs{})
	for _, arg := range args {
		dst.AddCloudImageMetadata(arg)
	}
	expectedParamsToService := transformMetadataArgsFromDescriptionToDomain(args)
	s.service.EXPECT().SaveMetadata(gomock.Any(), expectedParamsToService).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), dst)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Most validation is done through the mock, checking that the given parameters are correct.
}

// TestImportWithNonCustomSource verifies the behavior of the import operation when encountering metadata with non-custom sources.
// It creates several different metadata into the model and check that SaveMetadata is called with the right arguments,
// ie, including only custom sourced metadata.
func (s *importSuite) TestImportWithNonCustomSource(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	creationTime := time.Now()
	defaultArgs := description.CloudImageMetadataArgs{
		Stream:          "stream",
		Region:          "region",
		Version:         "version",
		Arch:            "arch",
		VirtType:        "virtType",
		RootStorageType: "rootStorageType",
		RootStorageSize: ptr(uint64(128)),
		DateCreated:     creationTime.UnixNano(),
		Source:          "source",
		Priority:        40,
		ImageId:         "attr0",
	}
	args := []description.CloudImageMetadataArgs{
		suffix(defaultArgs, 0, creationTime, customSource),
		suffix(defaultArgs, 1, creationTime),
		suffix(defaultArgs, 2, creationTime, customSource)}
	dst := description.NewModel(description.ModelArgs{})
	for _, arg := range args {
		dst.AddCloudImageMetadata(arg)
	}
	expectedParamsToService := transformMetadataArgsFromDescriptionToDomain(append(args[0:1], args[2])) // exclude non custom source
	s.service.EXPECT().SaveMetadata(gomock.Any(), expectedParamsToService).Return(nil)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), dst)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	// Most validation is done through the mock, checking that the given parameters are correct.
}

// TestImportFailureWhenSaveMetadata verifies that the import operation handles failure when saving cloud image metadata,
// returning the underlying failure.
func (s *importSuite) TestImportFailureWhenSaveMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	expectedError := errors.Errorf("error")
	dst := description.NewModel(description.ModelArgs{})
	// few args to trigger a save, value non-important
	for range 3 {
		dst.AddCloudImageMetadata(description.CloudImageMetadataArgs{})
	}
	s.service.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(expectedError)

	// Act
	op := s.newImportOperation(c)
	err := op.Execute(c.Context(), dst)

	// Assert
	c.Assert(err, tc.ErrorIs, expectedError)
}

// argsOpts is a type alias for a function that takes and returns a CloudImageMetadataArgs struct, enabling functional options.
type argsOpts func(arg description.CloudImageMetadataArgs) description.CloudImageMetadataArgs

// customSource sets the Source field of CloudImageMetadataArgs to CustomSource and returns the modified arguments.
func customSource(arg description.CloudImageMetadataArgs) description.CloudImageMetadataArgs {
	arg.Source = cloudimagemetadata.CustomSource
	return arg
}

// suffix returns a new cloudmetadataImageArgs, updating all fields by suffixing them with `_<i>` or adding some time
// to creation time. It allows to quickly generates different values for tests.
func suffix(arg description.CloudImageMetadataArgs, i int, t time.Time, argsOptions ...argsOpts) description.CloudImageMetadataArgs {
	rootStorageSize := uint64(i)
	if arg.RootStorageSize != nil {
		rootStorageSize = *arg.RootStorageSize + uint64(i)
	}
	result := description.CloudImageMetadataArgs{
		Stream:          fmt.Sprintf("%s_%d", arg.Stream, i),
		Region:          fmt.Sprintf("%s_%d", arg.Region, i),
		Version:         fmt.Sprintf("%s_%d", arg.Version, i),
		Arch:            fmt.Sprintf("%s_%d", arg.Arch, i),
		VirtType:        fmt.Sprintf("%s_%d", arg.VirtType, i),
		RootStorageType: fmt.Sprintf("%s_%d", arg.RootStorageType, i),
		RootStorageSize: &rootStorageSize,
		DateCreated:     t.Add(time.Duration(i) * time.Second).UnixNano(),
		Source:          fmt.Sprintf("%s_%d", arg.Source, i),
	}

	for _, opt := range argsOptions {
		result = opt(result)
	}

	return result
}

// transformMetadataArgsFromDescriptionToDomain is an helper function to convert CloudImageMetadataArgs to domain-specific Metadata.
func transformMetadataArgsFromDescriptionToDomain(args []description.CloudImageMetadataArgs) []cloudimagemetadata.Metadata {
	obtained := make([]cloudimagemetadata.Metadata, len(args))
	for i, m := range args {
		obtained[i] = cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          m.Stream,
				Region:          m.Region,
				Version:         m.Version,
				Arch:            m.Arch,
				VirtType:        m.VirtType,
				RootStorageType: m.RootStorageType,
				RootStorageSize: m.RootStorageSize,
				Source:          m.Source,
			},
			Priority:     m.Priority,
			ImageID:      m.ImageId,
			CreationTime: time.Unix(0, m.DateCreated),
		}
	}
	return obtained
}
