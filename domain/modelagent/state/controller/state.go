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
func (s State) GetControllerAgentVersionsByArchitecture(
	ctx context.Context,
	architectures []agentbinary.Architecture,
) ([]semversion.Number, error) {
	if len(architectures) == 0 {
		return []semversion.Number{}, nil
	}
	db, err := s.DB(ctx)
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	architectureIDs := make(ids, 0, len(architectures))
	for i, arch := range architectures {
		architectureIDs[i] = int(arch)
	}

	stmt, err := s.Prepare(`
SELECT version
FROM   controller_node_agent_version
WHERE  architecture_id IN ($ids[:])
`, architectureIDs)

	var versions []semversion.Number

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		iter := tx.Query(ctx, stmt, architectureIDs).Iter()
		defer iter.Close()

		for iter.Next() {
			var version string
			err := iter.Get(&version)
			if err != nil {
				return errors.Capture(err)
			}
			number, err := semversion.Parse(version)
			if err != nil {
				return errors.Capture(err)
			}
			versions = append(versions, number)
		}
		return nil
	})
	if err != nil {
		return []semversion.Number{}, errors.Capture(err)
	}

	return versions, nil
}
