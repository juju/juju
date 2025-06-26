// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"time"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/cloudimagemetadata/service"
	"github.com/juju/juju/domain/cloudimagemetadata/state"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, logger logger.Logger, clock clock.Clock) {
	coordinator.Add(&importOperation{
		logger: logger,
		clock:  clock,
	})
}

// importOperation represents a struct to handle the import operation for cloud image metadata.
type importOperation struct {
	modelmigration.BaseOperation

	service ImportService
	logger  logger.Logger
	clock   clock.Clock
}

// ImportService provides methods to save metadata information.
type ImportService interface {
	SaveMetadata(ctx context.Context, metadata []cloudimagemetadata.Metadata) error
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import cloud image metadata"
}

// Setup initializes the importOperation's service using the provided scope's ControllerDB, and sets up other dependencies.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(state.NewState(scope.ControllerDB(), i.clock, i.logger))
	return nil
}

// Execute performs the import operation for cloud image metadata defined in the given model.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	i.logger.Debugf(ctx, "importing cloudimagemetadata")

	images := model.CloudImageMetadata()
	metadata := make([]cloudimagemetadata.Metadata, 0, len(images))
	for _, image := range images {
		// We only want to import custom (user defined metadata).
		// Everything else *now* expires after a set time anyway and
		// coming from Juju < 2.3.4 would result in non-expiring metadata.
		if image.Source() != cloudimagemetadata.CustomSource {
			continue
		}
		var rootStoragePtr *uint64
		if rootStorageSize, ok := image.RootStorageSize(); ok {
			rootStoragePtr = &rootStorageSize
		}
		metadata = append(metadata, cloudimagemetadata.Metadata{
			MetadataAttributes: cloudimagemetadata.MetadataAttributes{
				Source:          image.Source(),
				Stream:          image.Stream(),
				Region:          image.Region(),
				Version:         image.Version(),
				Arch:            image.Arch(),
				RootStorageType: image.RootStorageType(),
				RootStorageSize: rootStoragePtr,
				VirtType:        image.VirtType(),
			},
			Priority:     image.Priority(),
			ImageID:      image.ImageId(),
			CreationTime: time.Unix(0, image.DateCreated()),
		})
	}
	err := i.service.SaveMetadata(ctx, metadata)
	if err != nil {
		return errors.Errorf("importing cloud image metadata: %w", err)
	}
	i.logger.Debugf(ctx, "importing cloudimagemetadata succeeded")
	return nil
}
