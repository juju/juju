// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	coremachine "github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/errors"
)

// State defines the access mechanism for interacting with authorized keys in
// the context of the model database.
type State struct {
	*domain.StateBase
}

// CheckMachineExists checks to see if the given machine exists in the model. If
// the machine does not exist an error satisfying
// [machineerrors.MachineNotFound] is returned.
func (s *State) CheckMachineExists(
	ctx context.Context,
	name coremachine.Name,
) error {
	db, err := s.DB(ctx)
	if err != nil {
		return errors.Errorf(
			"getting database to check if machine %q exists: %w",
			name, err,
		)
	}

	machineArg := machineName{name.String()}
	machineStmt, err := s.Prepare(`
SELECT &machineName.*
FROM machine
WHERE name = $machineName.name
`, machineArg)
	if err != nil {
		return errors.Errorf(
			"preparing statement for checking if machine %q exists: %w",
			name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, machineStmt, machineArg).Get(&machineArg)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"machine %q does not exist", name,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if machine %q exists: %w", name, err,
			)
		}
		return nil
	})

	if err != nil {
		return err
	}

	return nil
}

// GetModelUUID returns the uuid for the model represented by this state.
func (s *State) GetModelUUID(ctx context.Context) (model.UUID, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return model.UUID(""), errors.Errorf(
			"getting database to get the model uuid: %w", err,
		)
	}

	modelUUIDVal := modelUUIDValue{}

	stmt, err := s.Prepare(`
SELECT (uuid) AS (&modelUUIDValue.model_uuid)
FROM model
`, modelUUIDVal)
	if err != nil {
		return model.UUID(""), errors.Errorf(
			"preparing model uuid selection statement: %w", err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&modelUUIDVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"getting model uuid from database, read only model records don't exist",
			)
		} else if err != nil {
			return errors.Errorf(
				"getting model uuid from database: %w", err,
			)
		}
		return nil
	})

	if err != nil {
		return model.UUID(""), err
	}

	return model.UUID(modelUUIDVal.UUID), nil
}

// NamespaceForWatchUserAuthentication returns the namespace used to
// monitor user authentication changes.
func (s *State) NamespaceForWatchUserAuthentication() string {
	return "user_authentication"
}

// NamespaceForWatchModelAuthorizationKeys returns the namespace used to
// monitor authorization keys for the current model.
func (s *State) NamespaceForWatchModelAuthorizationKeys() string {
	return "model_authorized_keys"
}

// NewState constructs a new state for interacting with the underlying
// authorised keys of a model.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}
