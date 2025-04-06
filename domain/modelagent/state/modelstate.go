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
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
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

	if machineLife.LifeID == domainlife.Dead {
		return errors.Errorf("machine %q is dead", uuid).Add(machineerrors.MachineIsDead)
	}

	return nil
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(ctx context.Context, tx *sqlair.TX, ident unitUUID) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	stmt, err := st.Prepare(
		"SELECT &life.* FROM unit WHERE uuid = $unitUUID.uuid",
		ident,
		life{},
	)
	if err != nil {
		return errors.Errorf("preparing query for unit %q: %w", ident.UnitUUID, err)
	}

	var result life
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", ident.UnitUUID, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return applicationerrors.UnitIsDead
	default:
		return nil
	}
}

// GetMachineUUIDByName returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (st *State) GetMachineUUIDByName(ctx context.Context, name machine.Name) (string, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid machineUUID
	currentMachineName := machineName{Name: name.String()}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, uuid, currentMachineName)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		err := tx.Query(ctx, queryStmt, currentMachineName).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", name, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", name, err)
		}
		return nil
	})
	if err != nil {
		return uuid.UUID, errors.Errorf("getting UUID for machine %q: %w", name, err)
	}
	return uuid.UUID, nil
}

// GetMachineTargetAgentVersion returns the target agent version for the
// specified machine.
// The following error types can be expected:
// - [modelagenterrors.AgentVersionNotFound] when the agent version does not
// exist.
// - [machineerrors.MachineNotFound] when the machine specified by uuid  does
// not exists.
func (st *State) GetMachineTargetAgentVersion(ctx context.Context, uuid string) (coreagentbinary.Version, error) {
	db, err := st.DB()
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	info := machineTargetAgentVersionInfo{MachineUUID: uuid}

	stmt, err := st.Prepare(`
SELECT &machineTargetAgentVersionInfo.*
FROM v_machine_target_agent_version
WHERE machine_uuid = $machineTargetAgentVersionInfo.machine_uuid
`, info)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDead(ctx, tx, uuid)
		if errors.Is(err, machineerrors.MachineNotFound) {
			return errors.Errorf(
				"machine %q does not exist", uuid,
			).Add(machineerrors.MachineNotFound)
		} else if err != nil && !errors.Is(err, machineerrors.MachineIsDead) {
			return errors.Errorf(
				"checking machine %q exists: %w", uuid, err,
			)
		}

		err = tx.Query(ctx, stmt, info).Get(&info)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf(
				"machine %q has no target agent version set", uuid,
			).Add(modelagenterrors.AgentVersionNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting machine %q target agent version: %w",
				uuid, err,
			)
		}
		return nil
	})
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	vers, err := semversion.Parse(info.TargetVersion)
	if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"parsing machine %q agent version: %w",
			uuid, err,
		)
	}
	return coreagentbinary.Version{
		Number: vers,
		Arch:   info.ArchitectureName,
	}, nil
}

// GetUnitTargetAgentVersion returns the target agent version for the specified unit.
// The following error types can be expected:
// - [modelagenterrors.AgentVersionNotFound] - when the agent version does not exist.
func (st *State) GetUnitTargetAgentVersion(ctx context.Context, uuid coreunit.UUID) (coreagentbinary.Version, error) {
	db, err := st.DB()
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	info := unitAgentVersionInfo{UnitUUID: uuid}

	stmt, err := st.Prepare(`
SELECT av.target_version AS &unitAgentVersionInfo.target_version,
       a.name AS &unitAgentVersionInfo.architecture_name
FROM   agent_version AS av,
       unit_agent_version AS uav
JOIN   architecture AS a ON uav.architecture_id = a.id
WHERE  uav.unit_uuid = $unitAgentVersionInfo.unit_uuid
`, info)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, info).Get(&info)
		if errors.Is(err, sql.ErrNoRows) {
			return modelagenterrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf("getting unit agent version: %w", err)
		}
		return nil
	})
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	vers, err := semversion.Parse(info.TargetVersion)
	if err != nil {
		return coreagentbinary.Version{}, errors.Errorf("parsing unit agent version: %w", err)
	}
	return coreagentbinary.Version{
		Number: vers,
		Arch:   info.ArchitectureName,
	}, nil
}

// GetModelTargetAgentVersion returns the agent version for the model.
// If the agent_version table has no data,
// [modelagenterrors.AgentVersionNotFound] is returned.
func (st *State) GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
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
			return modelagenterrors.AgentVersionNotFound
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

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}
	unitName := unitName{Name: name.String()}

	query, err := st.Prepare(
		"SELECT &unitUUID.* FROM unit WHERE name = $unitName.name",
		unitUUID{},
		unitName,
	)
	if err != nil {
		return "", errors.Errorf("preparing query: %w", err)
	}

	unitUUID := unitUUID{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, query, unitName).Get(&unitUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("unit %q not found", name).Add(applicationerrors.UnitNotFound)
		}
		return err
	})
	if err != nil {
		return "", errors.Errorf("querying unit name: %w", err)
	}

	return unitUUID.UnitUUID, nil
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

	archMap := architectureMap{Name: version.Arch}

	archMapStmt, err := st.Prepare(`
SELECT id AS &architectureMap.id FROM architecture WHERE name = $architectureMap.name
`, archMap)
	if err != nil {
		return errors.Capture(err)
	}

	machineAgentVersion := machineAgentVersion{
		MachineUUID: machineUUID,
		Version:     version.Number.String(),
	}

	upsertRunningVersionStmt, err := st.Prepare(`
INSERT INTO machine_agent_version (*) VALUES ($machineAgentVersion.*)
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

// SetUnitRunningAgentBinaryVersion sets the running agent binary version for
// the provided unit uuid. Any previously set values for this unit uuid will be
// overwritten by this call.
//
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] if the unit does not exist.
// - [coreerrors.NotSupported] if the architecture is not known to the database.
func (st *State) SetUnitRunningAgentBinaryVersion(
	ctx context.Context,
	uuid coreunit.UUID,
	version coreagentbinary.Version,
) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	archMap := architectureMap{Name: version.Arch}

	archMapStmt, err := st.Prepare(`
SELECT id AS &architectureMap.id FROM architecture WHERE name = $architectureMap.name
`, archMap)
	if err != nil {
		return errors.Capture(err)
	}

	unitUUID := unitUUID{UnitUUID: uuid}

	unitAgentVersion := unitAgentVersion{
		UnitUUID: unitUUID.UnitUUID.String(),
		Version:  version.Number.String(),
	}

	upsertRunningVersionStmt, err := st.Prepare(`
INSERT INTO unit_agent_version (*)
VALUES ($unitAgentVersion.*)
ON CONFLICT (unit_uuid) DO
UPDATE SET version = excluded.version,
           architecture_id = excluded.architecture_id
`, unitAgentVersion)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		// Check if unit exists and is not dead.
		err := st.checkUnitNotDead(ctx, tx, unitUUID)
		if err != nil {
			return errors.Errorf(
				"checking unit %q exists: %w", uuid, err,
			)
		}

		// Look up architecture ID.
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

		unitAgentVersion.ArchtectureID = archMap.ID
		return tx.Query(ctx, upsertRunningVersionStmt, unitAgentVersion).Run()
	})

	if err != nil {
		return errors.Errorf(
			"setting running agent binary version for unit %q: %w",
			uuid, err,
		)
	}

	return nil
}
