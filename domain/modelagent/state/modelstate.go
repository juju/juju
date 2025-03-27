// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/internal/errors"
)

type State struct {
	*domain.StateBase
}

// NewState returns a new [State] object.
func NewState(factory database.TxnRunnerFactory) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// CheckMachineExists check to see if the given machine exists in the model.
// If the machine does not exist an error satisfying
// [machineerrors.MachineNotFound] is returned.
func (m *State) CheckMachineExists(
	ctx context.Context,
	name machine.Name,
) error {
	db, err := m.DB()
	if err != nil {
		return errors.Errorf(
			"getting database to check machine %q exists: %w",
			name, err,
		)
	}

	machineNameVal := machineName{name.String()}
	stmt, err := m.Prepare(`
SELECT &machineName.*
FROM machine
WHERE name = $machineName.name
`, machineNameVal)

	if err != nil {
		return errors.Errorf(
			"preparing machine %q selection statement: %w", name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineNameVal).Get(&machineNameVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"machine %q does not exist", name,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if machine %q exists: %w", name, err,
			)
		}

		return nil
	})

	return err
}

// CheckUnitExists checks to see if the given unit exists in the model. If
// the unit does not exist an error satisfying
// [applicationerrors.UnitNotFound] is returned.
func (m *State) CheckUnitExists(
	ctx context.Context,
	name string,
) error {
	db, err := m.DB()
	if err != nil {
		return errors.Errorf(
			"getting database to check unit %q exists: %w",
			name, err,
		)
	}

	unitNameVal := unitName{name}
	stmt, err := m.Prepare(`
SELECT &unitName.*
FROM unit
WHERE name = $unitName.name
`, unitNameVal)

	if err != nil {
		return errors.Errorf(
			"preparing unit %q selection statement: %w", name, err,
		)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, unitNameVal).Get(&unitNameVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"unit %q does not exist", name,
			).Add(applicationerrors.UnitNotFound)
		} else if err != nil {
			return errors.Errorf(
				"checking if unit %q exists: %w", name, err,
			)
		}

		return nil
	})

	return err
}

// GetTargetAgentVersion returns the agent version for the model.
// If the agent_version table has no data,
// [modelerrors.AgentVersionNotFound] is returned.
func (st *State) GetTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
	db, err := st.DB()
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	res := dbAgentVersion{}

	stmt, err := st.Prepare("SELECT &dbAgentVersion.target_version FROM agent_version", res)
	if err != nil {
		return semversion.Zero, errors.Errorf("preparing agent version query: %w", err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&res)
		if errors.Is(err, sql.ErrNoRows) {
			return modelerrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf("getting agent version: %w", err)
		}
		return nil
	})
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	vers, err := semversion.Parse(res.TargetAgentVersion)
	if err != nil {
		return semversion.Zero, errors.Errorf("parsing agent version: %w", err)
	}
	return vers, nil
}

// NamespaceForWatchAgentVersion returns the namespace identifier
// to watch for the agent version.
func (*State) NamespaceForWatchAgentVersion() string {
	return "agent_version"
}
