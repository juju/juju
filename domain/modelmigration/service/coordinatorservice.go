// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"
	"encoding/json"

	"github.com/juju/clock"
	"github.com/juju/names/v5"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/core/controller"
	"github.com/juju/juju/core/migration"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/user"
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
	// GetUserUUIDByName returns the UUID of the user.
	GetUserUUIDByName(ctx context.Context, user string) (user.UUID, error)
	// ModelMigrationInfo returns the information about the model in the
	// controller.
	ModelMigrationInfo(ctx context.Context, modelUUID model.UUID) (modelmigration.ModelMigrationInfo, error)
	// CreateMigration creates a migration record in the model state.
	CreateMigration(ctx context.Context, args modelmigration.CreateMigrationArgs) error
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
	if err := modelUUID.Validate(); err != nil {
		return errors.Errorf("invalid model UUID %w", err)
	}

	if !names.IsValidUser(initiatedBy.Id()) {
		return errors.Errorf("%w %q", modelmigrationerrors.InvalidUser, initiatedBy)
	}

	if err := targetInfo.Validate(); err != nil {
		return errors.Errorf("invalid target info %w", err)
	}

	// Check the user exists. If the user doesn't exist here, then we should not
	// proceed with the migration.
	userUUID, err := s.state.GetUserUUIDByName(ctx, initiatedBy.Id())
	if err != nil {
		return errors.Errorf("cannot get user UUID: %w", err)
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

	// Encode the macaroons, so that we can ensure that they are stored in the
	// state without modification.
	macaroons, err := encodeMaroons(targetInfo.Macaroons)
	if err != nil {
		return errors.Errorf("cannot encode macaroons: %w", err)
	}

	// Create the migration record in the model state.
	return s.state.CreateMigration(ctx, modelmigration.CreateMigrationArgs{
		ModelUUID:       modelUUID,
		UserUUID:        userUUID,
		ControllerUUID:  controllerUUID,
		ControllerAlias: targetInfo.ControllerAlias,
		Addrs:           targetInfo.Addrs,
		CACert:          targetInfo.CACert,
		Password:        targetInfo.Password,
		Macaroons:       macaroons,
	})
}

func encodeMaroons(macaroons []macaroon.Slice) ([]byte, error) {
	if len(macaroons) == 0 {
		return nil, nil
	}
	return json.Marshal(macaroons)
}
