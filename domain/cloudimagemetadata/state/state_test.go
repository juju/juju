// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"slices"
	stdtesting "testing"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/cloudimagemetadata/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite

	state *State
}

func TestStateSuite(t *stdtesting.T) { tc.Run(t, &stateSuite{}) }
func (s *stateSuite) SetUpTest(c *tc.C) {
	s.ControllerSuite.SetUpTest(c)
	s.state = NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))
}

// architecture represents a data model for a system architecture with a unique ID and name.
// It is used to prepare and execute sqlair statement.
type architecture struct {
	ID   int    `db:"id"`
	Name string `db:"name"`
}

func (s *stateSuite) TestArchitectureIDsByName(c *tc.C) {
	db, err := s.state.DB()
	c.Assert(err, tc.ErrorIsNil)

	var loadArchsStmt *sqlair.Statement
	loadArchsStmt, err = sqlair.Prepare(`SELECT &architecture.* FROM architecture;`, architecture{})
	c.Assert(err, tc.ErrorIsNil)

	var archs []architecture
	err = db.Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, loadArchsStmt).GetAll(&archs)
	})
	c.Assert(err, tc.ErrorIsNil)

	obtained := make(map[string]int, len(archs))
	for _, arch := range archs {
		obtained[arch.Name] = arch.ID
	}
	c.Assert(obtained, tc.DeepEquals, architectureIDsByName)
}

// TestSaveMetadata verifies that the metadata is saved correctly in the database
// and checks that creation time is set appropriately.
func (s *stateSuite) TestSaveMetadata(c *tc.C) {
	// Arrange
	testBeginTime := time.Now().Truncate(time.Second) // avoid truncate issue on dqlite creationTime check
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	expected := []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs, Priority: 42, ImageID: "42"},
	}

	//  Act
	err := s.state.SaveMetadata(c.Context(), expected)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtained, err := s.retrieveMetadataFromDB(c)
	for i := range obtained {
		c.Check(obtained[i].CreationTime, tc.After, testBeginTime)
		obtained[i].CreationTime = time.Time{} // ignore time since already checked.
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, expected)
}

// TestSaveMetadataWithDateCreated tests the SaveMetadata method by ensuring metadata is saved with the correct creation date.
func (s *stateSuite) TestSaveMetadataWithDateCreated(c *tc.C) {
	// Arrange
	testBeginTime := time.Now().UTC().Truncate(time.Second) // avoid truncate issue on dqlite creationTime check
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	expected := []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs, Priority: 42, ImageID: "42", CreationTime: testBeginTime},
	}

	//  Act
	err := s.state.SaveMetadata(c.Context(), expected)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtained, err := s.retrieveMetadataFromDB(c)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, expected)
}

// TestSaveMetadataSeveralMetadata verifies that multiple metadata entries are saved correctly in the database.
func (s *stateSuite) TestSaveMetadataSeveralMetadata(c *tc.C) {
	// Arrange
	testBeginTime := time.Now().Truncate(time.Second) // avoid truncate issue on dqlite creationTime check
	attrs1 := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arm64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	attrs2 := cloudimagemetadata.MetadataAttributes{
		Stream:          "chalk",
		Region:          "nether",
		Version:         "12.04",
		Arch:            "amd64",
		Source:          "test",
		RootStorageSize: ptr(uint64(1024)),
	}
	expected := []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs1, ImageID: "1"},
		{MetadataAttributes: attrs2, ImageID: "2"},
	}

	//  Act
	err := s.state.SaveMetadata(c.Context(), expected)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtained, err := s.retrieveMetadataFromDB(c)
	for i := range obtained {
		c.Check(obtained[i].CreationTime, tc.After, testBeginTime)
		obtained[i].CreationTime = time.Time{} // ignore time since already checked.
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, expected)
}

func (s *stateSuite) TestSaveMetadataUpdateMetadata(c *tc.C) {
	// Arrange
	testBeginTime := time.Now().Truncate(time.Second) // avoid truncate issue on dqlite creationTime check
	attrs1 := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arm64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	attrs2 := attrs1
	attrs2.RootStorageSize = ptr(uint64(1024)) // Not part of the key, but shouldn't be updated either

	//  Act
	err := s.state.SaveMetadata(c.Context(), []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs1, ImageID: "1"},
	})
	c.Assert(err, tc.ErrorIsNil)
	err = s.state.SaveMetadata(c.Context(), []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs2, ImageID: "2"},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtained, err := s.retrieveMetadataFromDB(c)
	for i := range obtained {
		c.Check(obtained[i].CreationTime, tc.After, testBeginTime)
		obtained[i].CreationTime = time.Time{} // ignore time since already checked.
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs1, ImageID: "2"}, // Imageid has been updated, but other attributes don't.
	})
}

func (s *stateSuite) TestSaveMetadataWithSameAttributes(c *tc.C) {
	// Arrange
	attrs1 := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arm64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	attrs2 := attrs1
	attrs2.RootStorageSize = ptr(uint64(1024)) // Not part of the key, but shouldn't be updated either

	//  Act
	err := s.state.SaveMetadata(c.Context(), []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs1, ImageID: "1"},
		{MetadataAttributes: attrs2, ImageID: "2"},
	})

	// Assert
	c.Assert(err, tc.ErrorIs, errors.ImageMetadataAlreadyExists)
}

// TestSaveMetadataSeveralMetadataWithInvalidArchitecture verifies that metadata with an invalid architecture is ignored
// when saving multiple metadata entries.
func (s *stateSuite) TestSaveMetadataSeveralMetadataWithInvalidArchitecture(c *tc.C) {
	// Arrange
	attrsBase := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "arm64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	attr1 := attrsBase
	attr2 := attrsBase
	attrIncorrectArch := attrsBase

	attr2.Region = "anotherRegion"
	attrIncorrectArch.Arch = "unknownArch"

	inserted := []cloudimagemetadata.Metadata{
		{MetadataAttributes: attr1, ImageID: "1"},
		{MetadataAttributes: attrIncorrectArch, ImageID: "2"},
		{MetadataAttributes: attr2, ImageID: "3"},
	}

	//  Act
	err := s.state.SaveMetadata(c.Context(), inserted)

	// Assert
	c.Assert(err, tc.ErrorMatches, ".*unknown architecture.*")
}

// TestDeleteMetadataWithImageID verifies that the DeleteMetadataWithImageID method correctly removes specified entries from the cloud_image_metadata table.
func (s *stateSuite) TestDeleteMetadataWithImageID(c *tc.C) {
	// Arrange
	s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid, created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a', datetime('now','localtime'), 'custom', 'stream', 'region-1', '22.04',0, 'virtType-test', 'rootStorageType-test', 42, 'to-keep'),
('b', datetime('now','localtime'), 'custom', 'stream', 'region-2', '22.04',0, 'virtType-test', 'rootStorageType-test', 3, 'to-delete'),
('c', datetime('now','localtime'), 'custom', 'stream', 'region-3', '22.04',0, 'virtType-test', 'rootStorageType-test', 42, 'to-delete')
`)

	//  Act
	err := s.state.DeleteMetadataWithImageID(c.Context(), "to-delete")

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	obtained, err := s.retrieveMetadataFromDB(c)
	for i := range obtained {
		obtained[i].CreationTime = time.Time{} // ignore time
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, []cloudimagemetadata.Metadata{
		{MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region-1",
			Version:         "22.04",
			Arch:            "amd64",
			VirtType:        "virtType-test",
			RootStorageType: "rootStorageType-test",
			Source:          "custom",
		}, Priority: 42, ImageID: "to-keep"},
	})
}

// TestFindMetadata tests the retrieval of metadata from the cloud_image_metadata table using various filters.
func (s *stateSuite) TestFindMetadata(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a',datetime('now','localtime'), 'non-custom', 'stream', 'region', '08.04',0, 'virtType', 'storage', 42, 'id'),
('b',datetime('now','localtime'), 'non-custom', 'unique', 'region', '10.04',1, 'virtType', 'storage', 42, 'id'),
('c',datetime('now','localtime'), 'non-custom', 'stream', 'unique', '12.04',2, 'virtType', 'storage', 42, 'id'),
('d',datetime('now','localtime'), 'non-custom', 'stream', 'region', '14.00',3, 'virtType', 'storage', 42, 'id'),
('e',datetime('now','localtime'), 'non-custom', 'stream', 'region', '16.04',4, 'virtType', 'storage', 42, 'id'),
('f',datetime('now','localtime'), 'non-custom', 'stream', 'region', '18.04',0, 'unique', 'storage', 42, 'id'),
('g',datetime('now','localtime'), 'non-custom', 'stream', 'region', '20.04',1, 'virtType', 'unique', 42, 'id'),
('h',datetime('now','localtime'), 'non-custom', 'stream', 'region', '22.04',2, 'virtType', 'storage', 1, 'id'),
('i',datetime('now','localtime'), 'non-custom', 'stream', 'region', '24.04',3, 'virtType', 'storage', 42, 'unique');
`)
	c.Assert(err, tc.ErrorIsNil)
	expectedBase, err := s.retrieveMetadataFromDB(c)
	c.Assert(err, tc.ErrorIsNil)

	for _, testCase := range []struct {
		description string
		filter      cloudimagemetadata.MetadataFilter
		accept      func(cloudimagemetadata.Metadata) bool
	}{
		{
			description: "Filter by region: 'unique'",
			filter:      cloudimagemetadata.MetadataFilter{Region: "unique"},
			accept:      func(metadata cloudimagemetadata.Metadata) bool { return metadata.Region == "unique" },
		},
		{
			description: "Filter by version: 'any of [22.04 24.04]'",
			filter:      cloudimagemetadata.MetadataFilter{Versions: []string{"22.04", "24.04"}},
			accept: func(metadata cloudimagemetadata.Metadata) bool {
				return slices.Contains([]string{"22.04", "24.04"}, metadata.Version)
			},
		},
		{
			description: "Filter by architecture: 'any of [amd64(id:0) arm64(id:1)]'",
			filter:      cloudimagemetadata.MetadataFilter{Arches: []string{"amd64", "arm64"}},
			accept: func(metadata cloudimagemetadata.Metadata) bool {
				return slices.Contains([]string{"amd64", "arm64"}, metadata.Arch)
			},
		},
		{
			description: "Filter by stream: 'unique'",
			filter:      cloudimagemetadata.MetadataFilter{Stream: "unique"},
			accept:      func(metadata cloudimagemetadata.Metadata) bool { return metadata.Stream == "unique" },
		},
		{
			description: "Filter by virt_type: 'unique'",
			filter:      cloudimagemetadata.MetadataFilter{VirtType: "unique"},
			accept:      func(metadata cloudimagemetadata.Metadata) bool { return metadata.VirtType == "unique" },
		},
		{
			description: "Filter by root_storage_type: 'unique'",
			filter:      cloudimagemetadata.MetadataFilter{RootStorageType: "unique"},
			accept:      func(metadata cloudimagemetadata.Metadata) bool { return metadata.RootStorageType == "unique" },
		},
		{
			description: "Filter by root_storage_type: 'storage' and region: 'region' and  stream: 'stream'",
			filter:      cloudimagemetadata.MetadataFilter{RootStorageType: "storage", Region: "region", Stream: "stream"},
			accept: func(metadata cloudimagemetadata.Metadata) bool {
				return metadata.RootStorageType == "storage" && metadata.Region == "region" && metadata.Stream == "stream"
			},
		},
	} {

		// Act
		obtained, err := s.state.FindMetadata(c.Context(), testCase.filter)
		c.Check(err, tc.ErrorIsNil, tc.Commentf("test: %s\n - unexpected error: %s", testCase.description, err))

		// Assert
		c.Check(obtained, tc.SameContents, filter(expectedBase, testCase.accept), tc.Commentf("test: %s\n - obtained value mismatched", testCase.description))
	}
}

// TestFindMetadataNotFound verifies that FindMetadata returns a NotFound error when no matching metadata is found.
func (s *stateSuite) TestFindMetadataNotFound(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a',datetime('now','localtime'), 'non-custom', 'stream', 'unique', '08.04',0, 'virtType', 'storage', 42, 'id'),
('b',datetime('now','localtime'), 'non-custom', 'unique', 'region', '10.04',1, 'virtType', 'storage', 42, 'id');
`)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	_, err = s.state.FindMetadata(c.Context(), cloudimagemetadata.MetadataFilter{Stream: "unique", Region: "unique"})

	// Assert
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

// TestFindMetadataExpired checks that non custom expired metadata entries are correctly excluded from query results.
func (s *stateSuite) TestFindMetadataExpired(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '08.04',0, 'virtType', 'storage', 42, 'id'),
('b',datetime('now','localtime'), 'non-custom', 'stream', 'region', '10.04',1, 'virtType', 'storage', 42, 'id'),
('c',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '12.04',1, 'virtType', 'storage', 42, 'id'),
('d',datetime('now','-2 days'), 'custom', 'stream', 'region', '14.04',1, 'virtType', 'storage', 42, 'id');
`)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtained, err := s.state.FindMetadata(c.Context(), cloudimagemetadata.MetadataFilter{})

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	for i := range obtained {
		obtained[i].CreationTime = time.Time{} // ignore time
	}
	c.Assert(obtained, tc.SameContents, []cloudimagemetadata.Metadata{{
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region",
			Version:         "10.04",
			Arch:            "arm64",
			VirtType:        "virtType",
			RootStorageType: "storage",
			Source:          "non-custom",
		},
		Priority: 42,
		ImageID:  "id",
	}, {
		MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region",
			Version:         "14.04",
			Arch:            "arm64",
			VirtType:        "virtType",
			RootStorageType: "storage",
			Source:          "custom",
		},
		Priority: 42,
		ImageID:  "id",
	},
	})
}

// TestAllCloudImageMetadata tests the retrieval of all cloud image metadata from the database,
// except expired one.
func (s *stateSuite) TestAllCloudImageMetadata(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '08.04',0, 'virtType', 'storage', 42, 'id'),
('b',datetime('now','localtime'), 'non-custom', 'stream', 'region', '10.04',1, 'virtType', 'storage', 42, 'id'),
('c',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '12.04',1, 'virtType', 'storage', 42, 'id'),
('d',datetime('now','localtime'), 'non-custom', 'stream', 'region', '16.04',1, 'virtType', 'storage', 42, 'id');
`)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtained, err := s.state.AllCloudImageMetadata(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	for i := range obtained {
		obtained[i].CreationTime = time.Time{} // ignore time
	}
	c.Assert(obtained, tc.SameContents, []cloudimagemetadata.Metadata{
		{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          "stream",
				Region:          "region",
				Version:         "10.04",
				Arch:            "arm64",
				VirtType:        "virtType",
				RootStorageType: "storage",
				Source:          "non-custom",
			},
			Priority: 42,
			ImageID:  "id",
		},
		{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          "stream",
				Region:          "region",
				Version:         "16.04",
				Arch:            "arm64",
				VirtType:        "virtType",
				RootStorageType: "storage",
				Source:          "non-custom",
			},
			Priority: 42,
			ImageID:  "id",
		},
	})
}

// TestAllCloudImageMetadataNoMetadata ensures that AllCloudImageMetadata returns no metadata
// without error if all entries have expired.
func (s *stateSuite) TestAllCloudImageMetadataNoMetadata(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,priority,image_id)
VALUES 
('a',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '08.04',0, 'virtType', 'storage', 42, 'id'),
('b',datetime('now','-3 days'), 'non-custom', 'stream', 'region', '12.04',1, 'virtType', 'storage', 42, 'id');
`)
	c.Assert(err, tc.ErrorIsNil)

	// Act
	obtained, err := s.state.AllCloudImageMetadata(c.Context())

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, []cloudimagemetadata.Metadata{}) // No non expired metadata
}

// TestCleanupMetatada verifies that the metadata is properly cleaned up on a new insert in the
// database.
// Custom source never expires.
func (s *stateSuite) TestCleanupMetatada(c *tc.C) {
	// Arrange
	err := s.runQuery(c, `
INSERT INTO cloud_image_metadata (uuid,created_at,source,stream,region,version,architecture_id,virt_type,root_storage_type,root_storage_size,priority,image_id)
VALUES 
('a',datetime('now','-3 days'), 'cloud', 'stream', 'region', '08.04',0, 'virtType', 'storage', 128, 42, 'id'),
('b',datetime('now','-3 days'), 'custom', 'stream', 'region', '12.04',1, 'virtType', 'storage', 1024, 42, 'id');
`)
	c.Assert(err, tc.ErrorIsNil)
	attrs := cloudimagemetadata.MetadataAttributes{
		Stream:          "stream",
		Region:          "region-test",
		Version:         "22.04",
		Arch:            "amd64",
		VirtType:        "virtType-test",
		RootStorageType: "rootStorageType-test",
		Source:          "test",
	}
	expected := []cloudimagemetadata.Metadata{
		{MetadataAttributes: attrs, Priority: 42, ImageID: "42"},
	}

	//  Act
	err = s.state.SaveMetadata(c.Context(), expected)
	c.Assert(err, tc.ErrorIsNil)

	// Assert
	obtained, err := s.retrieveMetadataFromDB(c)
	for i := range obtained {
		obtained[i].CreationTime = time.Time{} // ignore time for simplicity
	}
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(obtained, tc.SameContents, append(expected,
		// Custom that has not expired
		cloudimagemetadata.Metadata{MetadataAttributes: cloudimagemetadata.MetadataAttributes{
			Stream:          "stream",
			Region:          "region",
			Version:         "12.04",
			Arch:            "arm64",
			VirtType:        "virtType",
			RootStorageType: "storage",
			RootStorageSize: ptr(uint64(1024)),
			Source:          "custom",
		}, Priority: 42, ImageID: "id"}))
}
