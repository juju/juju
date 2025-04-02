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
// - [modelerrors.AgentVersionNotFound] when there is no target version found.
func (st *State) GetModelVersionInfo(ctx context.Context) (semversion.Number, bool, error) {
	db, err := st.DB()
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	res := dbAgentVersion{}
	versionStmt, err := st.Prepare("SELECT &dbAgentVersion.* FROM agent_version", res)
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	modelInfo := dbModel{}
	isControllerModelStmt, err := st.Prepare("SELECT &dbModel.* FROM model", modelInfo)
	if err != nil {
		return semversion.Zero, false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, versionStmt).Get(&res)
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf("getting currentagent version: %w", err)
		}
		err = tx.Query(ctx, isControllerModelStmt).Get(&modelInfo)
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
	return vers, modelInfo.IsControllerModel, nil
}

// SetTargetAgentVersion sets the target agent version and stream for the model.
func (st *State) SetTargetAgentVersion(ctx context.Context, targetVersion semversion.Number, agentStream *string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	vers := dbAgentVersion{
		TargetVersion: targetVersion.String(),
	}

	deleteStmt, err := st.Prepare(`DELETE FROM agent_version`)
	if err != nil {
		return errors.Capture(err)
	}

	insertStmt, err := st.Prepare(`INSERT INTO agent_version (target_version) VALUES ($dbAgentVersion.*)`, vers)
	if err != nil {
		return errors.Capture(err)
	}

	cfg := dbModelConfig{}
	streamStmt, err := st.Prepare(`
UPDATE model_config SET value = $dbModelConfig.value
WHERE  key = 'agent-stream'
`, cfg)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, deleteStmt).Run()
		if err != nil {
			return errors.Errorf("deleting old agent version: %w", err)
		}
		err = tx.Query(ctx, insertStmt, vers).Run()
		if err != nil {
			return errors.Errorf("setting agent version: %w", err)
		}

		if agentStream == nil {
			return nil
		}
		cfg.Value = *agentStream
		err = tx.Query(ctx, streamStmt, cfg).Run()
		if err != nil {
			return errors.Errorf("updating agent stream: %w", err)
		}

		return nil
	})
	return errors.Capture(err)
}
