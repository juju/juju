// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"context"

	"github.com/canonical/sqlair"

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
// all the controllers agent versions. It is used by upstream to further narrow
// down to get the highest version. Since we follow semantic versioning, doing a
// ORDER BY ASC won't give us the correct semantics.
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

	versions := make([]semversion.Number, len(agentVersions))
	for i, agent := range agentVersions {
		version, err := semversion.Parse(agent.Version)
		if err != nil {
			return []semversion.Number{}, errors.Capture(err)
		}
		versions[i] = version
	}

	return versions, nil
}
