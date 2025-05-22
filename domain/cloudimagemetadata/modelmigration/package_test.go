// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	stdtesting "testing"
	"time"

	"github.com/juju/description/v9"
	"github.com/juju/tc"

	"github.com/juju/juju/domain/cloudimagemetadata"
)

//go:generate go run go.uber.org/mock/mockgen -typed -package modelmigration -destination migrations_mock_test.go github.com/juju/juju/domain/cloudimagemetadata/modelmigration Coordinator,ImportService,ExportService

// transformMetadataFromDescriptionToDomain is a helper function to transform a slice of CloudImageMetadata
// from the description package to a slice of Metadata in the cloudimagemetadata package,
// mapping fields appropriately.
func transformMetadataFromDescriptionToDomain(metadata []description.CloudImageMetadata) []cloudimagemetadata.Metadata {
	obtained := make([]cloudimagemetadata.Metadata, len(metadata))
	for i, m := range metadata {
		var rootStorageSizePtr *uint64
		if rootStorageSize, hasSize := m.RootStorageSize(); hasSize {
			rootStorageSizePtr = &rootStorageSize
		}

		obtained[i] = cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          m.Stream(),
				Region:          m.Region(),
				Version:         m.Version(),
				Arch:            m.Arch(),
				VirtType:        m.VirtType(),
				RootStorageType: m.RootStorageType(),
				RootStorageSize: rootStorageSizePtr,
				Source:          m.Source(),
			},
			Priority:     m.Priority(),
			ImageID:      m.ImageId(),
			CreationTime: time.Unix(0, m.DateCreated()),
		}
	}
	return obtained
}
