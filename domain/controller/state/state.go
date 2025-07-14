// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
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

// GetControllerModelUUID returns the model UUID of the controller model.
func (st *State) GetControllerModelUUID(ctx context.Context) (model.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid controllerModelUUID
	stmt, err := st.Prepare(`
SELECT &controllerModelUUID.model_uuid
FROM   controller
`, uuid)
	if err != nil {
		return "", errors.Errorf("preparing select controller model uuid statement: %w", err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			// This should never reasonably happen.
			return errors.Errorf("internal error: controller model uuid not found")
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("getting controller model uuid: %w", err)
	}

	return model.UUID(uuid.UUID), nil
}

// GetStateServingInfo returns the state serving information.
func (st *State) GetStateServingInfo(ctx context.Context) (controller.StateServingInfo, error) {
	db, err := st.DB()
	if err != nil {
		return controller.StateServingInfo{}, errors.Capture(err)
	}
	var info controllerStateServingInfo
	stmt, err := st.Prepare(`SELECT &controllerStateServingInfo.* FROM controller`, info)
	if err != nil {
		return controller.StateServingInfo{}, errors.Errorf("preparing select state serving info statement: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&info)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("internal error: state serving info not found")
		}
		return err
	})
	if err != nil {
		return controller.StateServingInfo{}, errors.Errorf("getting state serving info: %w", err)
	}
	return controller.StateServingInfo{
		APIPort:        info.APIPort,
		Cert:           info.Cert,
		PrivateKey:     info.PrivateKey,
		CAPrivateKey:   info.CAPrivateKey,
		SystemIdentity: info.SystemIdentity,
	}, nil
}
