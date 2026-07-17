// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// State is the means by which the model agent accesses the controller's state.
type State struct {
	*domain.StateBase
}

// NewState returns a new [State] object.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetControllerAgentVersions has the responsibility of getting
// all the unique controller agent versions. It is used by upstream to further narrow
// down to get the highest running agent version.
// It returns an empty slice if no agent are found.
func (s *State) GetControllerAgentVersions(ctx context.Context) ([]semversion.Number, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	var agentVersions []agentVersion

	stmt, err := s.Prepare(`
SELECT &agentVersion.*
FROM   controller_node_agent_version
WHERE  version >= ''
GROUP  BY version
`, agentVersion{})
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&agentVersions)
		if err != nil && errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	versions := make([]semversion.Number, 0, len(agentVersions))
	for _, agent := range agentVersions {
		version, err := semversion.Parse(agent.Version)
		if err != nil {
			return []semversion.Number{}, errors.Capture(err)
		}
		versions = append(versions, version)
	}

	return versions, nil
}

// GetAllMachineTargetAgentVersionByArches returns all the given machine
// architectures for a given agent version that have an associated agent binary
// in the agent binary store.
func (st *State) GetAllMachineTargetAgentVersionByArches(
	ctx context.Context,
	version string,
) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	key := agentBinaryStore{
		Version: version,
	}

	stmt, err := st.Prepare(`
SELECT DISTINCT a.name AS &agentBinaryStore.architecture_name
FROM   agent_binary_store AS abs
JOIN   architecture AS a ON abs.architecture_id = a.id
WHERE  version = $agentBinaryStore.version 
`, key)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var found []agentBinaryStore
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, key).GetAll(&found)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf(
				"getting existing agent binaries for version %q: %w",
				version, err,
			)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(found, func(a agentBinaryStore) string {
		return a.ArchitectureName
	}), nil
}
