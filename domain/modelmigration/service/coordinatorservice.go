// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v5"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain/modelmigration"
	modelmigrationerrors "github.com/juju/juju/domain/modelmigration/errors"
	"github.com/juju/juju/internal/errors"
)

// CoordinatorService provides the means for coordinating model migration
// actions between controllers and answering questions about the underlying
// model(s) that are being migrated.
type CoordinatorService struct {
	state CoordinatorState

	clock clock.Clock
}

// CoordinatorState defines the interface required for accessing the underlying
// state of the model during migration.
type CoordinatorState interface {
	// ModelMigrationInfo returns the information about the model in the
	// controller.
	ModelMigrationInfo(ctx context.Context, modelUUID model.UUID) (modelmigration.ModelMigrationInfo, error)
	// CreateMigration creates a migration record in the model state.
	CreateMigration(ctx context.Context, initiatedBy names.UserTag, targetInfo migration.TargetInfo) error
}

// NewCoordinatorService is responsible for constructing a new
// [CoordinatorService] to handle model migration tasks.
func NewCoordinatorService(
	state CoordinatorState,
	clock clock.Clock,
) *CoordinatorService {
	return &CoordinatorService{
		state: state,
		clock: clock,
	}
}

// CreateMigration is responsible for creating a migration record in the state
// of the model that is being migrated.
// Returns [error.AlreadyExists] if a migration record already exists for the
// model.
func (s *CoordinatorService) CreateMigration(ctx context.Context, modelUUID model.UUID, initiatedBy names.UserTag, targetInfo migration.TargetInfo) error {
	if !names.IsValidUser(initiatedBy.Id()) {
		return errors.Errorf("%w %q", modelmigrationerrors.InvalidUser, initiatedBy)
	}

	if err := targetInfo.Validate(); err != nil {
		return errors.Errorf("invalid target info %w", err)
	}

	// Ensure that the controller UUID is valid.
	controllerUUID, err := controller.ParseUUID(targetInfo.ControllerTag.Id())
	if err != nil {
		return errors.Errorf("cannot parse controller UUID: %w", err)
	}

	// Check if the migration is even feasible before attempting to create a
	// migration record. The data here should be consistent with the data passed
	// in. If it is not, then we should not proceed with the migration.
	if info, err := s.state.ModelMigrationInfo(ctx, modelUUID); err != nil {
		return errors.Errorf("cannot get model info: %w", err)
	} else if info.IsControllerModel {
		return errors.Errorf("controller models can not be migrated")
	} else if info.ControllerUUID == controllerUUID {
		return errors.Errorf("nothing to migrate: model already located on target controller")
	} else if info.MigrationActive {
		return errors.Errorf("migration already in progress")
	}

	// Create the migration record in the model state.
	return s.state.CreateMigration(ctx, initiatedBy, targetInfo)
}
