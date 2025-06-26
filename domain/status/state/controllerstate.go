// Copyright 2025 Canonical Ltd.
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

// ControllerState represents the state of a single model within the controller's context.
// It provides access to information specific to the model, scoped by its UUID, within the controller database.
type ControllerState struct {
	*domain.StateBase

	// modelUUID is the uuid of the model that the controller state is scoped to.
	// It ensures that only data related to the specified model is accessible from this state.
	modelUUID coremodel.UUID
}

// NewControllerState returns a new [ControllerState] for interacting with the underlying controller state.
// modelUUID scopes the controller state to a specific model, allowing access to only the model's data from the controller database.
func NewControllerState(
	factory database.TxnRunnerFactory,
	modelUUID coremodel.UUID,
) *ControllerState {
	return &ControllerState{

		StateBase: domain.NewStateBase(factory),
		modelUUID: modelUUID,
	}
}

// GetModelStatusContext is responsible for returning a set of boolean indicators for
// key aspects about the current model so that the model's status can be derived from this
// information. If the model no longer exists for the provided UUID then an error
// satisfying [modelerrors.NotFound] will be returned.
func (s *ControllerState) GetModelStatusContext(ctx context.Context) (status.ModelStatusContext, error) {
	db, err := s.DB()
	if err != nil {
		return status.ModelStatusContext{}, errors.Capture(err)
	}

	mUUID := modelUUID{UUID: s.modelUUID.String()}
	var modelStatusCtxResult modelStatusContext

	stmt, err := s.Prepare(`
SELECT &modelStatusContext.*
FROM v_model_state
WHERE uuid = $modelUUID.uuid
`, modelStatusCtxResult, mUUID)
	if err != nil {
		return status.ModelStatusContext{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, mUUID).Get(&modelStatusCtxResult)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("model %q does not exist", mUUID.UUID).Add(modelerrors.NotFound)
		}
		return err
	})

	if err != nil {
		return status.ModelStatusContext{}, errors.Capture(err)
	}

	return status.ModelStatusContext{
		IsDestroying:                 modelStatusCtxResult.Destroying,
		IsMigrating:                  modelStatusCtxResult.Migrating,
		HasInvalidCloudCredential:    modelStatusCtxResult.CredentialInvalid,
		InvalidCloudCredentialReason: modelStatusCtxResult.CredentialInvalidReason,
	}, nil
}
