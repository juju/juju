// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
)

// State represents a type for interacting with the underlying state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying state.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// ControllerModelUUID returns the model UUID of the controller model.
func (st *State) ControllerModelUUID(ctx context.Context) (model.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}

	var uuid controllerModelUUID
	stmt, err := st.Prepare(`
SELECT &controllerModelUUID.model_uuid
FROM   controller
`, uuid)
	if err != nil {
		return "", errors.Annotate(err, "preparing select controller model uuid statement")
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			// This should never reasonably happen.
			return fmt.Errorf("internal error: controller model uuid not found")
		}
		return err
	})
	if err != nil {
		return "", errors.Annotate(err, "getting controller model uuid")
	}

	return model.UUID(uuid.UUID), nil
}

// GetModelActivationStatus returns the activation status of a model.
func (st *State) GetModelActivationStatus(ctx context.Context, controllerUUID string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	type controllerModel struct {
		UUID        model.UUID `db:"uuid"`
		Activated   bool       `db:"activated"`
		ModelTypeID int        `db:"model_type_id"`
		Name        string     `db:"name"`
		CloudUUID   string     `db:"cloud_uuid"`
		LifeID      int        `db:"life_id"`
		OwnerUUID   string     `db:"owner_uuid"`
	}

	m := controllerModel{
		UUID: model.UUID(controllerUUID),
	}

	stmt, err := st.Prepare(`
SELECT &controllerModel.*
FROM   model
WHERE  uuid = $controllerModel.uuid
`, controllerModel{})

	if err != nil {
		return false, errors.Annotate(err, "preparing select model activated status statement")
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, m).Get(&m)
	})

	return m.Activated, err
}
