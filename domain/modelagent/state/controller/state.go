// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"
	"maps"
	"slices"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/application/architecture"
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

// GetMissingMachineTargetAgentVersionByArches returns the missing
// architectures that do not have agent binaries for the given target
// version from the provided set of architectures.
func (st *State) GetMissingMachineTargetAgentVersionByArches(
	ctx context.Context,
	version string,
	arches map[architecture.Architecture]struct{},
) (map[architecture.Architecture]struct{}, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	key := agentBinaryStore{
		Version: version,
	}
	archIDs := architectures(slices.Collect(maps.Keys(arches)))

	stmt, err := st.Prepare(`
SELECT &agentBinaryStore.*
FROM   agent_binary_store
WHERE  version = $agentBinaryStore.version 
AND    architecture_id IN ($architectures[:])
`, key, archIDs)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var found []agentBinaryStore
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, key, archIDs).GetAll(&found)
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

	// Removed the found architectures from the input set. The resulting
	// set is the missing architectures.
	result := make(map[architecture.Architecture]struct{})
	maps.Copy(result, arches)
	for _, abs := range found {
		delete(result, architecture.Architecture(abs.ArchitectureID))
	}

	return result, nil
}
