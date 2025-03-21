// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"database/sql"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/domain/cloudimagemetadata"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	"github.com/juju/juju/internal/errors"
)

type bootstrapSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&bootstrapSuite{})

func (s *bootstrapSuite) TestAddCustomImageMetadata(c *gc.C) {

	defaultStream := "defaulted"
	metadata := []*imagemetadata.ImageMetadata{
		{
			Id:          "Id-1",
			Storage:     "Storage-1",
			VirtType:    "VirtType-1",
			Arch:        "arm64",
			Version:     "Version-1",
			RegionAlias: "RegionAlias-1",
			RegionName:  "RegionName-1",
			Endpoint:    "Endpoint-1",
			Stream:      "Stream-1",
		},
		{
			Id:          "Id-2",
			Storage:     "Storage-2",
			VirtType:    "VirtType-2",
			Arch:        "amd64",
			Version:     "Version-2",
			RegionAlias: "RegionAlias-2",
			RegionName:  "RegionName-2",
			Endpoint:    "Endpoint-2",
			// No stream => defaulted
		},
		{
			Id:          "Id-3",
			Storage:     "Storage-3",
			VirtType:    "VirtType-3",
			Arch:        "arm64",
			Version:     "Version-3",
			RegionAlias: "RegionAlias-3",
			RegionName:  "RegionName-3",
			Endpoint:    "Endpoint-3",
			Stream:      "Stream-3",
		},
	}
	err := AddCustomImageMetadata(clock.WallClock, defaultStream, metadata)(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	insertedMetadata, err := s.retrieveMetadataFromDB()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(insertedMetadata, jc.SameContents,
		[]cloudimagemetadata.Metadata{
			{
				MetadataAttributes: cloudimagemetadata.MetadataAttributes{
					Stream:          "Stream-1",
					Region:          "RegionName-1",
					Version:         "Version-1",
					Arch:            "arm64",
					VirtType:        "VirtType-1",
					RootStorageType: "Storage-1",
					Source:          cloudimagemetadata.CustomSource,
				},
				Priority: simplestreams.CUSTOM_CLOUD_DATA,
				ImageID:  "Id-1",
			},
			{
				MetadataAttributes: cloudimagemetadata.MetadataAttributes{
					Stream:          "defaulted", // defaulted
					Region:          "RegionName-2",
					Version:         "Version-2",
					Arch:            "amd64",
					VirtType:        "VirtType-2",
					RootStorageType: "Storage-2",
					Source:          cloudimagemetadata.CustomSource,
				},
				Priority: simplestreams.CUSTOM_CLOUD_DATA,
				ImageID:  "Id-2",
			},
			{
				MetadataAttributes: cloudimagemetadata.MetadataAttributes{
					Stream:          "Stream-3",
					Region:          "RegionName-3",
					Version:         "Version-3",
					Arch:            "arm64",
					VirtType:        "VirtType-3",
					RootStorageType: "Storage-3",
					Source:          cloudimagemetadata.CustomSource,
				},
				Priority: simplestreams.CUSTOM_CLOUD_DATA,
				ImageID:  "Id-3",
			},
		},
	)
}

func (s *bootstrapSuite) TestInitCustomImageMetadataWithNil(c *gc.C) {
	err := AddCustomImageMetadata(clock.WallClock, "useless", []*imagemetadata.ImageMetadata{nil, nil, nil})(context.Background(), s.TxnRunner(), s.NoopTxnRunner())
	c.Assert(err, jc.ErrorIsNil)

	insertedMetadata, err := s.retrieveMetadataFromDB()
	c.Assert(err, jc.ErrorIsNil)
	c.Check(insertedMetadata, gc.HasLen, 0)
}

// retrieveMetadataFromDB retrieves all metadata from the cloud_image_metadata database table.
// It joins the architecture table to fetch architecture-related details and returns the metadata slice.
//
// It is used in test to keep save and find tests independent of each other
func (s *bootstrapSuite) retrieveMetadataFromDB() ([]cloudimagemetadata.Metadata, error) {
	var metadata []cloudimagemetadata.Metadata
	return metadata, s.TxnRunner().StdTxn(context.Background(), func(ctx context.Context, tx *sql.Tx) error {
		rows, err := tx.Query(`
SELECT 
source,
stream,
region,
version,
virt_type,
root_storage_type,
priority,
arch.name as archName,
image_id
 FROM cloud_image_metadata
 JOIN architecture arch on cloud_image_metadata.architecture_id = arch.id`)
		if err != nil {
			return errors.Capture(err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var dbMetadata cloudimagemetadata.Metadata
			if err = rows.Scan(
				&dbMetadata.Source,
				&dbMetadata.Stream,
				&dbMetadata.Region,
				&dbMetadata.Version,
				&dbMetadata.VirtType,
				&dbMetadata.RootStorageType,
				&dbMetadata.Priority,
				&dbMetadata.Arch,
				&dbMetadata.ImageID,
			); err != nil {
				return errors.Capture(err)
			}
			metadata = append(metadata, dbMetadata)
		}
		return errors.Capture(rows.Err())
	})
}
