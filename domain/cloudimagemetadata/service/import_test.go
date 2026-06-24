// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"testing"
	"time"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/collections/set"
	"github.com/juju/tc"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/testhelpers"
)

type importSuite struct {
	testhelpers.IsolationSuite
	state *MockState
}

func TestImportSuite(t *testing.T) {
	tc.Run(t, &importSuite{})
}

func (s *importSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *importSuite) envelopeRow() coremodelmigration.CloudImageMetadata {
	size := uint64(128)
	return coremodelmigration.CloudImageMetadata{
		Stream:          "released",
		Region:          "us-east-1",
		Version:         "24.04",
		Arch:            "amd64",
		VirtType:        "hvm",
		RootStorageType: "ebs",
		RootStorageSize: &size,
		Source:          cloudimagemetadata.CustomSource,
		Priority:        10,
		ImageID:         "ami-123",
		CreatedAt:       time.Now().UTC(),
	}
}

// TestImportCloudImageMetadata verifies the import is non-destructive: it
// validates and delegates to the compare-or-insert state method (never the
// upsert SaveMetadata).
func (s *importSuite) TestImportCloudImageMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	row := s.envelopeRow()
	expected := []cloudimagemetadata.Metadata{{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          row.Stream,
			Region:          row.Region,
			Version:         row.Version,
			Arch:            row.Arch,
			VirtType:        row.VirtType,
			RootStorageType: row.RootStorageType,
			RootStorageSize: row.RootStorageSize,
			Source:          row.Source,
		},
		Priority:     row.Priority,
		ImageID:      row.ImageID,
		CreationTime: row.CreatedAt,
	}}
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().CompareOrInsertMetadata(gomock.Any(), expected).Return(nil, nil)

	conflicts, err := NewService(s.state).ImportCloudImageMetadata(c.Context(), []coremodelmigration.CloudImageMetadata{row})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conflicts, tc.HasLen, 0)
}

// TestImportCloudImageMetadataConflict verifies natural-key conflicts reported
// by the state are passed back to the caller (for a non-fatal warning).
func (s *importSuite) TestImportCloudImageMetadataConflict(c *tc.C) {
	defer s.setupMocks(c).Finish()

	row := s.envelopeRow()
	conflict := cloudimagemetadata.MetadataConflict{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream: row.Stream, Region: row.Region, Version: row.Version, Arch: row.Arch,
			VirtType: row.VirtType, RootStorageType: row.RootStorageType, Source: row.Source,
		},
		ExistingImageID: "ami-target",
		IncomingImageID: "ami-123",
	}
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().CompareOrInsertMetadata(gomock.Any(), gomock.Any()).
		Return([]cloudimagemetadata.MetadataConflict{conflict}, nil)

	conflicts, err := NewService(s.state).ImportCloudImageMetadata(c.Context(), []coremodelmigration.CloudImageMetadata{row})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conflicts, tc.DeepEquals, []cloudimagemetadata.MetadataConflict{conflict})
}

func (s *importSuite) TestImportCloudImageMetadataEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	conflicts, err := NewService(s.state).ImportCloudImageMetadata(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conflicts, tc.HasLen, 0)
}

func (s *importSuite) TestImportCloudImageMetadataError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().CompareOrInsertMetadata(gomock.Any(), gomock.Any()).Return(nil, expected)

	_, err := NewService(s.state).ImportCloudImageMetadata(c.Context(), []coremodelmigration.CloudImageMetadata{s.envelopeRow()})
	c.Assert(err, tc.ErrorIs, expected)
}
