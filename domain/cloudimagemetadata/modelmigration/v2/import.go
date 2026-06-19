// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package v2

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	coremodelmigration "github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/cloudimagemetadata/service"
	"github.com/juju/juju/domain/cloudimagemetadata/state"
	"github.com/juju/juju/internal/errors"
)

// ImportCloudImageMetadata recreates the model's custom cloud image
// metadata rows on the target controller.
func ImportCloudImageMetadata(
	ctx context.Context, controllerDB database.TxnRunnerFactory, clock clock.Clock, logger logger.Logger,
	rows []coremodelmigration.CloudImageMetadata,
) error {
	if len(rows) == 0 {
		return nil
	}

	svc := service.NewService(state.NewState(controllerDB, clock, logger))
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
	if err := svc.SaveMetadata(ctx, metadata); err != nil {
		return errors.Errorf("saving cloud image metadata: %w", err)
	}
	return nil
}
