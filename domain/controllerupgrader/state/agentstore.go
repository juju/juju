// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/agentbinary"
	"github.com/juju/juju/domain/controllerupgrader/internal"
	"github.com/juju/juju/internal/errors"
)

func prepareAgentStoreBinaries(state *domain.StateBase) (*sqlair.Statement, error) {
	stmt, err := state.Prepare(`
SELECT abs.version AS &agentStoreBinary.version,
       abs.architecture_id AS &agentStoreBinary.architecture_id
FROM agent_binary_store AS abs`, agentStoreBinary{})
	if err != nil {
		return nil, errors.Errorf("preparing get all agent binaries query: %w", err)
	}
	return stmt, nil
}

func getAgentStoreBinaries(
	ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement,
) ([]agentStoreBinary, error) {
	var binaries []agentStoreBinary
	err := tx.Query(ctx, stmt).GetAll(&binaries)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, nil
	}
	return binaries, err
}

func decodeAgentStoreBinaries(rows []agentStoreBinary) ([]internal.AgentStoreBinary, error) {
	binaries := make([]internal.AgentStoreBinary, 0, len(rows))
	for _, row := range rows {
		version, err := semversion.Parse(row.Version)
		if err != nil {
			return nil, errors.Errorf(
				"parsing agent binary version %q: %w", row.Version, err,
			)
		}
		binaries = append(binaries, internal.AgentStoreBinary{
			Version:      version,
			Architecture: agentbinary.Architecture(row.ArchitectureID),
		})
	}
	return binaries, nil
}
