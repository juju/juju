// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	modelerrors "github.com/juju/juju/domain/model/errors"
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

// GetModelAgentVersion returns the agent version for the specified model. If
// the agent version cannot be found, an error satisfying [errors.NotFound]
// will be returned.
func (st *State) GetModelAgentVersion(ctx context.Context, modelID model.UUID) (version.Number, error) {
	db, err := st.DB()
	if err != nil {
		return version.Zero, errors.Trace(err)
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
		return version.Zero, fmt.Errorf("preparing agent version query for model with ID %q: %w", modelID, err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt, args).Get(&rval)
	})

	if errors.Is(err, sql.ErrNoRows) {
		return version.Zero, fmt.Errorf("%w for id %q", modelerrors.NotFound, modelID)
	} else if err != nil {
		return version.Zero, fmt.Errorf("cannot get agent version for model with ID %q: %w", modelID, domain.CoerceError(err))
	}

	vers, err := version.Parse(rval.TargetAgentVersion)
	if err != nil {
		return version.Zero, fmt.Errorf("cannot parse agent version %q for model with ID %q: %w", rval.TargetAgentVersion, modelID, err)
	}
	return vers, nil
}
