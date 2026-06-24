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

func (s *importSuite) TestImportCloudImageMetadata(c *tc.C) {
	defer s.setupMocks(c).Finish()

	size := uint64(128)
	createdAt := time.Now().UTC()
	expected := []cloudimagemetadata.Metadata{{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          "released",
			Region:          "us-east-1",
			Version:         "24.04",
			Arch:            "amd64",
			VirtType:        "hvm",
			RootStorageType: "ebs",
			RootStorageSize: &size,
			Source:          cloudimagemetadata.CustomSource,
		},
		Priority:     10,
		ImageID:      "ami-123",
		CreationTime: createdAt,
	}}
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().SaveMetadata(gomock.Any(), expected).Return(nil)

	err := NewService(s.state).ImportCloudImageMetadata(c.Context(), []coremodelmigration.CloudImageMetadata{{
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
		CreatedAt:       createdAt,
	}})
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportCloudImageMetadataEmpty(c *tc.C) {
	defer s.setupMocks(c).Finish()

	err := NewService(s.state).ImportCloudImageMetadata(c.Context(), nil)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *importSuite) TestImportCloudImageMetadataError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	expected := errors.New("boom")
	s.state.EXPECT().SupportedArchitectures(gomock.Any()).Return(set.NewStrings("amd64"))
	s.state.EXPECT().SaveMetadata(gomock.Any(), gomock.Any()).Return(expected)

	err := NewService(s.state).ImportCloudImageMetadata(c.Context(), []coremodelmigration.CloudImageMetadata{{
		Stream:  "released",
		Region:  "us-east-1",
		Version: "24.04",
		Arch:    "amd64",
		Source:  cloudimagemetadata.CustomSource,
		ImageID: "ami-123",
	}})
	c.Assert(err, tc.ErrorIs, expected)
}
