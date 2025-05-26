// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// ControllerState represents a type for interacting with the underlying model state.
type ControllerState struct {
	*domain.StateBase

	modelUUID coremodel.UUID
}

// NewControllerState returns a new ControllerState for interacting with the underlying model state.
func NewControllerState(
	factory database.TxnRunnerFactory,
	modelUUID coremodel.UUID,
) *ControllerState {
	return &ControllerState{
		StateBase: domain.NewStateBase(factory),
		modelUUID: modelUUID,
	}
}

// GetModelState is responsible for returning a set of boolean indicators for
// key aspects about a model so that a model's status can be derived from this
// information. If no model exists for the provided UUID then an error
// satisfying [modelerrors.NotFound] will be returned.
func (s *ControllerState) GetModelState(ctx context.Context) (status.ModelState, error) {
	db, err := s.DB()
	if err != nil {
		return status.ModelState{}, errors.Capture(err)
	}

	uuid := s.modelUUID
	modelUUIDVal := dbModelUUID{UUID: uuid.String()}
	modelState := dbModelState{}

	stmt, err := s.Prepare(`
SELECT &dbModelState.* FROM v_model_state WHERE uuid = $dbModelUUID.uuid
`, modelUUIDVal, modelState)
	if err != nil {
		return status.ModelState{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, modelUUIDVal).Get(&modelState)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("model does not exist").Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return status.ModelState{}, errors.Errorf(
			"getting model %q state: %w", uuid, err,
		)
	}

	return status.ModelState{
		Destroying:                   modelState.Destroying,
		Migrating:                    modelState.Migrating,
		HasInvalidCloudCredential:    modelState.CredentialInvalid,
		InvalidCloudCredentialReason: modelState.CredentialInvalidReason,
	}, nil
}
