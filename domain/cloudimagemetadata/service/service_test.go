// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/collections/set"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/cloudimagemetadata"
	cloudimageerrors "github.com/juju/juju/domain/cloudimagemetadata/errors"
	"github.com/juju/juju/internal/errors"
)

type serviceSuite struct {
	testing.IsolationSuite
	state *MockState
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

// TestSaveMetadataSuccess verifies that metadata is saved successfully without errors.
func (s *serviceSuite) TestSaveMetadataSuccess(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	inserted := []cloudimagemetadata.Metadata{{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Version: "1.2.3",
			Stream:  "stream",
			Source:  "source",
			Arch:    "amd64",
			Region:  "region",
		},
		ImageID: "not-dead-beaf",
	}}
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().SaveMetadata(gomock.Any(), inserted).Return(nil)

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(), inserted)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

// TestSaveMetadataEmptyImageID verifies that the SaveMetadata function returns an error when given metadata with an empty ImageID.
func (s *serviceSuite) TestSaveMetadataEmptyImageID(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	// State layer shouldn't be called

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(), []cloudimagemetadata.Metadata{{ImageID: ""}})

	// Assert
	c.Assert(err, jc.ErrorIs, cloudimageerrors.NotValid)
	c.Assert(err, jc.ErrorIs, cloudimageerrors.EmptyImageID)
	c.Assert(err, gc.ErrorMatches, "image id is empty: invalid metadata")
}

// TestSaveMetadataInvalidFields validates that SaveMetadata returns an error when required fields in metadata are missing.
func (s *serviceSuite) TestSaveMetadataInvalidFields(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	// State layer shouldn't be called

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(), []cloudimagemetadata.Metadata{{ImageID: "dead-beaf" /* some field are required */}})

	// Assert
	c.Assert(err, jc.ErrorIs, cloudimageerrors.NotValid)
	c.Assert(err, gc.ErrorMatches, "missing version, stream, source, arch, region: invalid metadata for image dead-beaf")
}

// TestSaveMetadataEmptyInsert verifies that SaveMetadata returns no errors when inserting empty data
func (s *serviceSuite) TestSaveMetadataEmptyInsert(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	// State layer shouldn't be called

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(), nil /* empty array */)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

// TestSaveMetadataInvalidArchitectureName verifies that the SaveMetadata method returns an error when given an unsupported architecture.
func (s *serviceSuite) TestSaveMetadataInvalidArchitectureName(c *gc.C) { // Arrange
	// Arrange
	defer s.setupMocks(c).Finish()

	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64", "arm64"))

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(),
		[]cloudimagemetadata.Metadata{{ImageID: "dead-beaf",
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Version: "1.2.3",
				Stream:  "stream",
				Source:  "source",
				Arch:    "risc",
				Region:  "region",
			}}},
	)

	// Assert
	c.Assert(err, jc.ErrorIs, cloudimageerrors.NotValid)
	c.Assert(err, gc.ErrorMatches, "unsupported architecture risc \\(should be any of \\[(amd64 arm64|arm64 amd64)\\]\\): invalid metadata")
}

// TestSaveMetadataError tests the SaveMetadata method to ensure it return all other unexpected errors from underlying
// state.
func (s *serviceSuite) TestSaveMetadataError(c *gc.C) { // Arrange
	// Arrange
	defer s.setupMocks(c).Finish()
	errExpected := errors.New("oh no!!")
	validMetadata := []cloudimagemetadata.Metadata{{ImageID: "dead-beaf",
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Version: "1.2.3",
			Stream:  "stream",
			Source:  "source",
			Arch:    "amd64",
			Region:  "region",
		}}}
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().SaveMetadata(gomock.Any(), validMetadata).Return(errExpected)

	// Act
	err := NewService(s.state).SaveMetadata(context.Background(), validMetadata)

	// Assert
	c.Assert(err, jc.ErrorIs, errExpected)
}

// TestDeleteMetadataSuccess verifies that deleting metadata by image ID calls the right function in the underlying state.
func (s *serviceSuite) TestDeleteMetadataSuccess(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	s.state.EXPECT().DeleteMetadataWithImageID(gomock.Any(), "dead-beaf").Return(nil)

	// Act
	err := NewService(s.state).DeleteMetadataWithImageID(context.Background(), "dead-beaf")

	// Assert
	c.Assert(err, jc.ErrorIsNil)
}

// TestDeleteMetadataEmptyImageID tests that trying to delete metadata with an empty image ID causes a EmptyImageID error
func (s *serviceSuite) TestDeleteMetadataEmptyImageID(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	// Act
	err := NewService(s.state).DeleteMetadataWithImageID(context.Background(), "")

	// Assert
	c.Assert(err, jc.ErrorIs, cloudimageerrors.EmptyImageID)
}

// TestDeleteMetadataError verifies that the DeleteMetadataWithImageID method returns the underlying error when deletion fails.
func (s *serviceSuite) TestDeleteMetadataError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()
	errExpected := errors.New("oh no!!")
	s.state.EXPECT().DeleteMetadataWithImageID(gomock.Any(), "dead-beaf").Return(errExpected)

	// Act
	err := NewService(s.state).DeleteMetadataWithImageID(context.Background(), "dead-beaf")

	// Assert
	c.Assert(err, jc.ErrorIs, errExpected)
}

// TestFindMetadataSuccessOneSource is a unit test that verifies the FindMetadata method returns metadata grouped by source
// when it successfully fetches data matching the given criteria from a single source.
func (s *serviceSuite) TestFindMetadataSuccessOneSource(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	criteria := cloudimagemetadata.MetadataFilter{
		Region:   "region",
		Versions: []string{"1.2.3"},
		Arches:   []string{"amd64"},
		Stream:   "custom",
	}

	metadata1 := cloudimagemetadata.Metadata{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Region:  "region",
			Version: "1.2.3",
			Arch:    "amd64",
			Source:  "source",
		},
		ImageID: "id",
	}
	s.state.EXPECT().FindMetadata(gomock.Any(), criteria).Return([]cloudimagemetadata.Metadata{
		metadata1, metadata1, metadata1,
	}, nil)

	// Act
	result, err := NewService(s.state).FindMetadata(context.Background(), criteria)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, map[string][]cloudimagemetadata.Metadata{
		"source": {metadata1, metadata1, metadata1},
	})
}

// TestFindMetadataSuccessOneSource is a unit test that verifies the FindMetadata method returns metadata grouped by source
// when it successfully fetches data matching the given criteria from several sources.
func (s *serviceSuite) TestFindMetadataSuccessSeveralSources(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	criteria := cloudimagemetadata.MetadataFilter{
		Region:   "region",
		Versions: []string{"1.2.3"},
		Arches:   []string{"amd64"},
		Stream:   "custom",
	}

	metadata1 := cloudimagemetadata.Metadata{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Region:  "region",
			Version: "1.2.3",
			Arch:    "amd64",
			Source:  "source",
		},
		ImageID: "id",
	}
	metadataAlt := metadata1
	metadataAlt.Source = "alt"

	s.state.EXPECT().FindMetadata(gomock.Any(), criteria).Return([]cloudimagemetadata.Metadata{
		metadataAlt, metadata1, metadata1, metadataAlt, metadata1,
	}, nil)

	// Act
	result, err := NewService(s.state).FindMetadata(context.Background(), criteria)

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, map[string][]cloudimagemetadata.Metadata{
		"source": {metadata1, metadata1, metadata1},
		"alt":    {metadataAlt, metadataAlt},
	})
}

// TestFindMetadataNotFound test that a notFound error is returned when the metadata is not found based on the given criteria.
func (s *serviceSuite) TestFindMetadataNotFound(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	criteria := cloudimagemetadata.MetadataFilter{Region: "whatever"}

	s.state.EXPECT().FindMetadata(gomock.Any(), criteria).Return(nil, cloudimageerrors.NotFound)

	// Act
	_, err := NewService(s.state).FindMetadata(context.Background(), criteria)

	// Assert
	c.Assert(err, jc.ErrorIs, cloudimageerrors.NotFound)
}

// TestFindMetadataError tests the behavior of the service's FindMetadata method when an error is returned by the state.
// The error should be returned.
func (s *serviceSuite) TestFindMetadataError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	errExpected := errors.New("oh no!!")
	criteria := cloudimagemetadata.MetadataFilter{Region: "whatever"}

	s.state.EXPECT().FindMetadata(gomock.Any(), criteria).Return(nil, errExpected)

	// Act
	_, err := NewService(s.state).FindMetadata(context.Background(), criteria)

	// Assert
	c.Assert(err, jc.ErrorIs, errExpected)
}

// TestAllCloudImageMetadataSuccess verifies that the AllCloudImageMetadata function successfully retrieves metadata.
func (s *serviceSuite) TestAllCloudImageMetadataSuccess(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	metadata1 := cloudimagemetadata.Metadata{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Region:  "region",
			Version: "1.2.3",
			Arch:    "amd64",
			Source:  "source",
		},
		ImageID: "id",
	}
	expected := []cloudimagemetadata.Metadata{metadata1, metadata1, metadata1}
	s.state.EXPECT().AllCloudImageMetadata(gomock.Any()).Return(expected, nil)

	// Act
	result, err := NewService(s.state).AllCloudImageMetadata(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.DeepEquals, expected)
}

// TestAllCloudImageMetadataError tests that AllCloudImageMetadata returns the underlying error when the to state fails.
func (s *serviceSuite) TestAllCloudImageMetadataError(c *gc.C) {
	// Arrange
	defer s.setupMocks(c).Finish()

	errExpected := errors.New("oh no!!")

	s.state.EXPECT().AllCloudImageMetadata(gomock.Any()).Return(nil, errExpected)

	// Act
	_, err := NewService(s.state).AllCloudImageMetadata(context.Background())

	// Assert
	c.Assert(err, jc.ErrorIs, errExpected)
}
