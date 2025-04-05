// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// State is used to access the database.
type State struct {
	*domain.StateBase
}

// NewState creates a state to access the database.
func NewState(factory coredatabase.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetModelVersionInfo returns the current model target version
// and whether the model is the controller model or not.
// The following errors can be expected:
// - [modeleerrors.NotFound] when the model does not exist.
func (st *State) GetModelVersionInfo(ctx context.Context) (semversion.Number, bool, error) {
	db, err := st.DB()
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	res := dbModelInfo{}
	stmt, err := st.Prepare("SELECT &dbModelInfo.* FROM agent_version, model", res)
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&res)
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.NotFound
		} else if err != nil {
			return errors.Errorf("getting model info: %w", err)
		}
		return nil
	})
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	vers, err := semversion.Parse(res.TargetVersion)
	if err != nil {
		return semversion.Zero, false, errors.Errorf("parsing agent version: %w", err)
	}
	return vers, res.IsControllerModel, nil
}
