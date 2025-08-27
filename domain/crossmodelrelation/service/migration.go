// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/trace"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/internal/errors"
)

// ModelDBState describes retrieval and persistence methods for cross model
// relations in the model database.
type ModelMigrationDBState interface {
	// ImportOffers adds offers being migrated to the current model.
	ImportOffers(context.Context, []crossmodelrelation.OfferImport) error
}

// MigrationService provides the API for model migration actions within
// the cross model relation domain.
type MigrationService struct {
	modelState ModelMigrationDBState
	logger     logger.Logger
}

// MigrationService returns a new service reference wrapping the input state
// for migration.
func NewMigrationService(
	modelState ModelMigrationDBState,
	logger logger.Logger,
) *MigrationService {
	return &MigrationService{
		modelState: modelState,
		logger:     logger,
	}
}

// ImportOffers adds offers being migrated to the current model.
func (s *MigrationService) ImportOffers(ctx context.Context, imports []crossmodelrelation.OfferImport) error {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	return errors.Capture(s.modelState.ImportOffers(ctx, imports))
}
