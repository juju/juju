// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/cloudimagemetadata/state"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/environs/simplestreams"
	internaldatabase "github.com/juju/juju/internal/database"
)

// AddCustomImageMetadata creates a BootstrapOpt function that adds custom image metadata to the database.
func AddCustomImageMetadata(clock clock.Clock, defaultStream string, metadata []*imagemetadata.ImageMetadata) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, _ database.TxnRunner) error {
		defaultMetadata := make([]cloudimagemetadata.Metadata, 0, len(metadata))
		for _, m := range metadata {
			if m == nil {
				continue
			}
			stream := m.Stream
			if stream == "" {
				stream = defaultStream
			}
			defaultMetadata = append(defaultMetadata, cloudimagemetadata.Metadata{
				MetadataAttributes: cloudimagemetadata.MetadataAttributes{
					Stream:          stream,
					Region:          m.RegionName,
					Version:         m.Version,
					Arch:            m.Arch,
					VirtType:        m.VirtType,
					RootStorageType: m.Storage,
					Source:          cloudimagemetadata.CustomSource,
				},
				Priority: simplestreams.CUSTOM_CLOUD_DATA,
				ImageID:  m.Id,
			})
		}

		return state.InsertMetadata(ctx, controller, defaultMetadata, clock.Now())
	}
}
