// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	"github.com/juju/juju/domain/life"
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

// checkMachineNotDead checks if the machine with the given uuid exists and that
// its current life status is not one of dead. This is meant as a helper func
// to assert that a machine can be operated on inside of a transaction.
// The following errors can be expected:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [machineerrors.MachineIsDead] if the machine is dead.
func (st *State) checkMachineNotDead(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) error {
	machineLife := machineLife{UUID: uuid}
	stmt, err := st.Prepare(`
SELECT &machineLife.* FROM machine WHERE uuid = $machineLife.uuid
`, machineLife)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, machineLife).Get(&machineLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("machine %q does not exist", uuid).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return errors.Errorf(
			"checking if machine %q exists: %w",
			uuid, err,
		)
	}

	if machineLife.LifeID == life.Dead {
		return errors.Errorf("machine %q is dead", uuid).Add(machineerrors.MachineIsDead)
	}

	return nil
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

// SetMachineRunningAgentBinaryVersion sets the running agent binary version for
// the provided machine uuid. Any previously set values for this machine uuid
// will be overwritten by this call.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [machineerrors.MachineIsDead] if the machine is dead.
// - [coreerrors.NotSupported] if the architecture is not known to the database.
func (st *State) SetMachineRunningAgentBinaryVersion(
	ctx context.Context,
	machineUUID string,
	version coreagentbinary.Version,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	type ArchitectureMap struct {
		ID   int    `db:"id"`
		Name string `db:"name"`
	}
	archMap := ArchitectureMap{Name: version.Arch}

	archMapStmt, err := st.Prepare(`
SELECT id AS &ArchitectureMap.id FROM architecture WHERE name = $ArchitectureMap.name
`, archMap)
	if err != nil {
		return errors.Capture(err)
	}

	type MachineAgentVersion struct {
		MachineUUID    string `db:"machine_uuid"`
		Version        string `db:"version"`
		ArchitectureID int    `db:"architecture_id"`
	}
	machineAgentVersion := MachineAgentVersion{
		MachineUUID: machineUUID,
		Version:     version.Number.String(),
	}

	upsertRunningVersionStmt, err := st.Prepare(`
INSERT INTO machine_agent_version (*) VALUES ($MachineAgentVersion.*)
ON CONFLICT (machine_uuid) DO
UPDATE SET version = excluded.version, architecture_id = excluded.architecture_id
`, machineAgentVersion)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDead(ctx, tx, machineUUID)
		if err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, archMapStmt, archMap).Get(&archMap)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"architecture %q is unsupported", version.Arch,
			).Add(coreerrors.NotSupported)
		} else if err != nil {
			return errors.Errorf(
				"looking up id for architecture %q: %w", version.Arch, err,
			)
		}

		machineAgentVersion.ArchitectureID = archMap.ID
		return tx.Query(ctx, upsertRunningVersionStmt, machineAgentVersion).Run()
	})

	if err != nil {
		return errors.Errorf(
			"setting running agent binary version for machine %q: %w",
			machineUUID, err,
		)
	}

	return nil
}
