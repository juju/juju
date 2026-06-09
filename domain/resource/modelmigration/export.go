// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/domain/resource"
	"github.com/juju/juju/domain/resource/state"
	"github.com/juju/juju/internal/errors"
)

// ExportState provides the resource state methods needed for model migration
// export.
type ExportState interface {
	// ListAllModelResources returns the application and unit resources to export
	// for all applications in the model.
	ListAllModelResources(ctx context.Context) (resource.ExportedResources, error)
}

// Exporter provides resource export methods for model migration.
type Exporter struct {
	resourceState ExportState
}

// NewExporter returns a new resource model migration exporter.
func NewExporter(
	factory database.TxnRunnerFactory,
	clock clock.Clock,
	logger logger.Logger,
) *Exporter {
	return &Exporter{
		resourceState: state.NewState(factory, clock, logger),
	}
}

// ExportResources returns the model resource references that need binary
// transfer during model migration.
func (e *Exporter) ExportResources(ctx context.Context) (resource.ExportedResources, error) {
	resources, err := e.resourceState.ListAllModelResources(ctx)
	return resources, errors.Capture(err)
}
