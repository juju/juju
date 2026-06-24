// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/internal/errors"
)

// ImportCloudImageMetadata recreates the model's custom cloud image metadata
// rows on the target controller from the v8 migration envelope.
//
// It is called directly by the v8 migration import driver in
// internal/migration.
func (s Service) ImportCloudImageMetadata(
	ctx context.Context, rows []coremodelmigration.CloudImageMetadata,
) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	if len(rows) == 0 {
		return nil
	}

	metadata := make([]cloudimagemetadata.Metadata, 0, len(rows))
	for _, r := range rows {
		metadata = append(metadata, cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Stream:          r.Stream,
				Region:          r.Region,
				Version:         r.Version,
				Arch:            r.Arch,
				VirtType:        r.VirtType,
				RootStorageType: r.RootStorageType,
				RootStorageSize: r.RootStorageSize,
				Source:          r.Source,
			},
			Priority:     r.Priority,
			ImageID:      r.ImageID,
			CreationTime: r.CreatedAt,
		})
	}
	if err := s.SaveMetadata(ctx, metadata); err != nil {
		return errors.Errorf("saving cloud image metadata: %w", err)
	}
	return nil
}
