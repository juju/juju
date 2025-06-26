// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v10"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/domain/cloudimagemetadata"
	"github.com/juju/juju/domain/cloudimagemetadata/service"
	"github.com/juju/juju/domain/cloudimagemetadata/state"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(coordinator Coordinator, logger logger.Logger, clock clock.Clock) {
	coordinator.Add(&exportOperation{
		clock:  clock,
		logger: logger,
	})
}

// ExportService provides methods to export cloud image metadata.
type ExportService interface {
	AllCloudImageMetadata(ctx context.Context) ([]cloudimagemetadata.Metadata, error)
}

// exportOperation describes a way to execute a migration for
// exporting external controllers.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService
	logger  logger.Logger
	clock   clock.Clock
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export cloud image metadata"
}

// Setup initializes the exportOperation by creating a new service instance using the provided
// database transaction factory and logger.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewService(state.NewState(scope.ControllerDB(), e.clock, e.logger))
	return nil
}

// Execute retrieves cloud image metadata and adds it to the provided model.
// If the metadata retrieval fails, it returns an error.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	e.logger.Debugf(ctx, "exporting cloudimagemetadata")

	metadata, err := e.service.AllCloudImageMetadata(ctx)
	if err != nil {
		return errors.Errorf("exporting cloud image metadata: %w", err)
	}
	for _, m := range metadata {
		model.AddCloudImageMetadata(description.CloudImageMetadataArgs{
			Stream:          m.Stream,
			Region:          m.Region,
			Version:         m.Version,
			Arch:            m.Arch,
			VirtType:        m.VirtType,
			RootStorageType: m.RootStorageType,
			RootStorageSize: m.RootStorageSize,
			DateCreated:     m.CreationTime.UnixNano(),
			Source:          m.Source,
			Priority:        m.Priority,
			ImageId:         m.ImageID,
		})
	}

	e.logger.Debugf(ctx, "exporting cloudimagemetadata succeeded")
	return nil
}
