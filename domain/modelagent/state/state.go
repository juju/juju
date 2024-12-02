// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

// State is responsible for retrieving a model's running agent version from
// the database.
type State struct {
	*domain.StateBase
}

// NewState returns a new State object.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetModelTargetAgentVersion returns the agent version for the specified model.
// If the agent version cannot be found, an error satisfying
// [modelerrors.NotFound] will be returned.
func (st *State) GetModelTargetAgentVersion(ctx context.Context, modelID model.UUID) (version.Number, error) {
	db, err := st.DB()
	if err != nil {
		return version.Zero, errors.Capture(err)
	}

	q := `
SELECT &dbAgentVersion.target_agent_version
FROM v_model
WHERE uuid = $M.model_id
`

	rval := dbAgentVersion{}
	args := sqlair.M{
		"model_id": modelID,
	}

	stmt, err := st.Prepare(q, rval, args)
	if err != nil {
		return version.Zero, errors.Errorf("preparing agent version query for model with ID %q: %w", modelID, err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, args).Get(&rval)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("%w for id %q", modelerrors.NotFound, modelID)
		} else if err != nil {
			return errors.Errorf("cannot get agent version for model with ID %q: %w", modelID, err)
		}
		return nil
	})
	if err != nil {
		return version.Zero, errors.Capture(err)
	}

	vers, err := version.Parse(rval.TargetAgentVersion)
	if err != nil {
		return version.Zero, errors.Errorf("cannot parse agent version %q for model with ID %q: %w", rval.TargetAgentVersion, modelID, err)
	}
	return vers, nil
}
