// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
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

// GetControllerAgentVersionsByArchitecture has the responsibility of getting
// all the controllers agent versions. It is used by upstream to further narrow
// down to get the highest version. Since we follow semantic versioning, doing a
// ORDER BY ASC won't give us the correct semantics.
func (s *State) GetControllerAgentVersionsByArchitecture(
	ctx context.Context,
	architectures []agentbinary.Architecture,
) (map[agentbinary.Architecture][]semversion.Number, error) {
	zeroArchAndVersions := map[agentbinary.Architecture][]semversion.Number{}
	if len(architectures) == 0 {
		return zeroArchAndVersions, nil
	}
	db, err := s.DB(ctx)
	if err != nil {
		return zeroArchAndVersions, errors.Capture(err)
	}

	architectureIDs := make(ids, len(architectures))
	for i, arch := range architectures {
		architectureIDs[i] = int(arch)
	}

	var agentVersions []agentVersionArchitecture

	stmt, err := s.Prepare(`
SELECT &agentVersionArchitecture.*
FROM   controller_node_agent_version
WHERE  architecture_id IN ($ids[:])
`, agentVersionArchitecture{}, architectureIDs)
	if err != nil {
		return zeroArchAndVersions, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, architectureIDs).GetAll(&agentVersions)
		if err != nil && errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("no controller agents found")
		} else if err != nil {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return zeroArchAndVersions, errors.Capture(err)
	}

	versions := make(map[agentbinary.Architecture][]semversion.Number, len(architectures))
	for _, architecture := range architectures {
		versions[architecture] = []semversion.Number{}
	}

	for _, agent := range agentVersions {
		version, err := semversion.Parse(agent.Version)
		if err != nil {
			return zeroArchAndVersions, errors.Capture(err)
		}
		archID := agentbinary.Architecture(agent.ArchitectureID)
		versions[archID] = append(versions[archID], version)
	}

	return versions, nil
}
