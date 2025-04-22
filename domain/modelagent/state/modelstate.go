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

// checkMachineExists checks if the machine with the given uuid exists. This is
// meant as a helper func to assert that a machine can be operated on inside
// of a transaction. True or false is returned indicating if the machine exists.
func (st *State) checkMachineExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid string,
) (bool, error) {
	machineUUID := machineUUID{UUID: uuid}
	stmt, err := st.Prepare(`
SELECT &machineUUID.* FROM machine WHERE uuid = $machineUUID.uuid
`, machineUUID)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, machineUUID).Get(&machineUUID)
	if errors.Is(err, sqlair.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
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

// checkUnitExists check if the unit exists. True or false is returned
// indicating this fact.
func (st *State) checkUnitExists(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coreunit.UUID,
) (bool, error) {
	unitUUID := unitUUID{UnitUUID: uuid}

	stmt, err := st.Prepare(
		"SELECT &unitUUID.* FROM unit WHERE uuid = $unitUUID.uuid",
		unitUUID,
	)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt, unitUUID).Get(&unitUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	} else if err != nil {
		return false, errors.Capture(err)
	}

	return true, nil
}

// checkUnitNotDead checks if the unit exists and is not dead. It's possible to
// access alive and dying units, but not dead ones:
// - If the unit is not found, [applicationerrors.UnitNotFound] is returned.
// - If the unit is dead, [applicationerrors.UnitIsDead] is returned.
func (st *State) checkUnitNotDead(
	ctx context.Context,
	tx *sqlair.TX,
	uuid coreunit.UUID,
) error {
	type life struct {
		LifeID domainlife.Life `db:"life_id"`
	}

	unitUUID := unitUUID{UnitUUID: uuid}

	stmt, err := st.Prepare(
		"SELECT &life.* FROM unit WHERE uuid = $unitUUID.uuid",
		unitUUID,
		life{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	var result life
	err = tx.Query(ctx, stmt, unitUUID).Get(&result)
	if errors.Is(err, sql.ErrNoRows) {
		return applicationerrors.UnitNotFound
	} else if err != nil {
		return errors.Errorf("checking unit %q exists: %w", uuid, err)
	}

	switch result.LifeID {
	case domainlife.Dead:
		return applicationerrors.UnitIsDead
	default:
		return nil
	}
}

// GetMachinesAgentBinaryMetadata reports the agent binary metadata that each
// machine in the model is currently running. This is a bulk call to support
// operations such as model export where it is expected that the state of a
// model stays relatively static over the operation. This function will never
// provide enough granuality into what machine fails as part of the checks.
//
// The following errors can be expected:
// - [modelagenterrors.MachineAgentVersionNotSet] when one or more machines in
// the model do not have their agent version set.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't exist
// for one or more machines in the model.
func (st *State) GetMachinesAgentBinaryMetadata(
	ctx context.Context,
) (map[machine.Name]coreagentbinary.Metadata, error) {
	// As of writing we do not maintain a strong RI between the agent
	// binary that a machine should be running and an agent binary in the
	// model's store. To do this we would need to start refactoring how machines
	// work and that is to record the intent with which a machine is provisioned.
	// i.e we would need to start caching the fact that we expect machine x to
	// use version y with agent binaries z.
	//
	// This would also require actively getting agent binaries from external sources
	// when creating machines. This is currently done lazily.

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    mav.name AS &machineAgentBinaryMetadata.name,
          mav.version AS &machineAgentBinaryMetadata.version,
          mav.architecture_name AS &machineAgentBinaryMetadata.architecture_name,
          osm.size AS &machineAgentBinaryMetadata.size,
          osm.sha_256 AS &machineAgentBinaryMetadata.sha_256,
          osm.sha_384 AS &machineAgentBinaryMetadata.sha_384
FROM      v_machine_agent_version AS mav
LEFT JOIN v_agent_binary_store AS abs ON (
          mav.version = abs.version
AND       mav.architecture_id = abs.architecture_id)
LEFT JOIN object_store_metadata AS osm ON abs.object_store_uuid = osm.uuid
`, machineAgentBinaryMetadata{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineCount := rowCount{}
	stmtMachineCount, err := st.Prepare(`
SELECT (count(*)) AS (&rowCount.count)
FROM   machine
`, machineCount)
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineBinaryMetadata := []machineAgentBinaryMetadata{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&machineBinaryMetadata)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting machine binary information from database: %w", err,
			)
		}

		if err := tx.Query(ctx, stmtMachineCount).Get(&machineCount); err != nil {
			return errors.Errorf(
				"getting the number of machines currently in the model: %w", err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(machineBinaryMetadata) != machineCount.Count {
		return nil, errors.New(
			"not all machines in the model have their agent version set",
		).Add(modelagenterrors.MachineAgentVersionNotSet)
	}

	rval := make(map[machine.Name]coreagentbinary.Metadata, len(machineBinaryMetadata))
	for _, machineRecord := range machineBinaryMetadata {
		// Because we are performing a left join against agent binary store with
		// no RI there exists the possibility that machine might be using a
		// version that isn't in the model store. In theory this should never
		// happen. We need to check all of the agent binary store values from
		// the query for null to work out if this case exists.
		//
		// In theory just checking one of these values should be enough to
		// identify the condition but all three are done for safety and
		// correctness.
		if !machineRecord.SHA256.Valid ||
			!machineRecord.SHA384.Valid ||
			!machineRecord.Size.Valid {
			return nil, errors.Errorf(
				"machine %q has missing agent binaries in the model",
				machineRecord.MachineName,
			).Add(modelagenterrors.MissingAgentBinaries)
		}

		number, err := semversion.Parse(machineRecord.Version)
		if err != nil {
			return nil, errors.Errorf(
				"parsing machine %q version %q number: %w",
				machineRecord.MachineName, machineRecord.Version, err,
			)
		}

		machineName := machine.Name(machineRecord.MachineName)
		rval[machineName] = coreagentbinary.Metadata{
			SHA256: machineRecord.SHA256.String,
			SHA384: machineRecord.SHA384.String,
			Size:   machineRecord.Size.Int64,
			Version: coreagentbinary.Version{
				Number: number,
				Arch:   machineRecord.Architecture,
			},
		}
	}

	return rval, nil
}

// GetMachinesNotAtTargetAgentVersion returns the list of machines where
// their agent version is not the same as the models target agent version or
// who have no agent version reproted at all. If no machines exist that match
// this criteria an empty slice is returned.
func (st *State) GetMachinesNotAtTargetAgentVersion(
	ctx context.Context,
) ([]machine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT &machineName.*
FROM v_machine_target_agent_version
WHERE version != target_version
UNION
SELECT name
FROM machine
WHERE uuid NOT IN (SELECT machine_uuid
                   FROM v_machine_target_agent_version)
`

	queryStmt, err := st.Prepare(query, machineName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := []machineName{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&rval)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Errorf(
			"getting all machine names that are not at the target agent version: %w",
			err,
		)
	}

	names := make([]machine.Name, 0, len(rval))
	for _, mNameVal := range rval {
		names = append(names, machine.Name(mNameVal.Name))
	}
	return names, nil
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

// GetMachineRunningAgentBinaryVersion reports the currently set agent binary
// version value for a machine. The following errors can be expected:
// - [machineerrors.MachineNotFound] when the machine being asked for does
// not exist.
// - [modelagenterrors.AgentVersionNotFound] when no
// running agent version has been set for the given machine.
func (st *State) GetMachineRunningAgentBinaryVersion(
	ctx context.Context,
	uuid string,
) (coreagentbinary.Version, error) {
	db, err := st.DB()
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	rval := machineAgentVersionInfo{}
	machineUUID := machineUUIDRef{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &machineAgentVersionInfo.*
FROM v_machine_agent_version
WHERE machine_uuid = $machineUUIDRef.machine_uuid
`, rval, machineUUID)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkMachineExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking machine %q exists: %w", uuid, err,
			)
		} else if !exists {
			return errors.Errorf(
				"machine %q does not exist", uuid,
			).Add(machineerrors.MachineNotFound)
		}

		err = tx.Query(ctx, stmt, machineUUID).Get(&rval)
		if errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf(
				"machine %q has no agent version set", uuid,
			).Add(modelagenterrors.AgentVersionNotFound)
		} else if err != nil {
			return errors.Errorf(
				"getting machine %q agent version: %w",
				uuid, err,
			)
		}
		return nil
	})
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	vers, err := semversion.Parse(rval.Version)
	if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"parsing machine %q agent version: %w",
			uuid, err,
		)
	}
	return coreagentbinary.Version{
		Number: vers,
		Arch:   rval.Architecture,
	}, nil
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

	info := machineTargetAgentVersionInfo{}
	machineUUID := machineUUIDRef{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &machineTargetAgentVersionInfo.*
FROM v_machine_target_agent_version
WHERE machine_uuid = $machineUUIDRef.machine_uuid
`, info, machineUUID)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkMachineExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking machine %q exists: %w", uuid, err,
			)
		} else if !exists {
			return errors.Errorf(
				"machine %q does not exist", uuid,
			).Add(machineerrors.MachineNotFound)
		}

		err = tx.Query(ctx, stmt, machineUUID).Get(&info)
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

// GetUnitsAgentBinaryMetadata reports the agent binary metadata that each
// unit in the model is currently running. This is a bulk call to support
// operations such as model export where it is expected that the state of a
// model stays relatively static over the operation. This function will never
// provide enough granuality into what unit fails as part of the checks.
//
// The following errors can be expected:
// - [modelagenterrors.UnitAgentVersionNotSet] when one or more units in
// the model do not have their agent version set.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't exist
// for one or more units in the model.
func (st *State) GetUnitsAgentBinaryMetadata(
	ctx context.Context,
) (map[coreunit.Name]coreagentbinary.Metadata, error) {
	// As of writing we do not maintain a strong RI between the agent
	// binary that a unit should be running and an agent binary in the
	// model's store. To do this we would need to start refactoring how units
	// work and that is to record the intent with which a unit is provisioned.
	// i.e we would need to start caching the fact that we expect unit x to
	// use version y with agent binaries z.
	//
	// This would also require actively getting agent binaries from external sources
	// when creating units. This is currently done lazily.

	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT    u.name AS &unitAgentBinaryMetadata.name,
          uav.version AS &unitAgentBinaryMetadata.version,
          a.name AS &unitAgentBinaryMetadata.architecture_name,
          osm.size AS &unitAgentBinaryMetadata.size,
          osm.sha_256 AS &unitAgentBinaryMetadata.sha_256,
          osm.sha_384 AS &unitAgentBinaryMetadata.sha_384
FROM      unit_agent_version AS uav
JOIN      unit AS u ON uav.unit_uuid = u.uuid
JOIN      architecture AS a ON uav.architecture_id = a.id
LEFT JOIN v_agent_binary_store AS abs ON (
          uav.version = abs.version
AND       uav.architecture_id = abs.architecture_id)
LEFT JOIN object_store_metadata AS osm ON abs.object_store_uuid = osm.uuid
`, unitAgentBinaryMetadata{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitCount := rowCount{}
	stmtUnitCount, err := st.Prepare(`
SELECT (count(*)) AS (&rowCount.count)
FROM   unit
`, unitCount)
	if err != nil {
		return nil, errors.Capture(err)
	}

	unitBinaryMetadata := []unitAgentBinaryMetadata{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&unitBinaryMetadata)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"getting unit binary information from database: %w", err,
			)
		}

		if err := tx.Query(ctx, stmtUnitCount).Get(&unitCount); err != nil {
			return errors.Errorf(
				"getting the number of units currently in the model: %w", err,
			)
		}

		return nil
	})

	if err != nil {
		return nil, errors.Capture(err)
	}

	if len(unitBinaryMetadata) != unitCount.Count {
		return nil, errors.New(
			"not all units in the model have their agent version set",
		).Add(modelagenterrors.UnitAgentVersionNotSet)
	}

	rval := make(map[coreunit.Name]coreagentbinary.Metadata, len(unitBinaryMetadata))
	for _, unitRecord := range unitBinaryMetadata {
		// Because we are performing a left join against agent binary store with
		// no RI there exists the possibility that unit might be using a
		// version that isn't in the model store. In theory this should never
		// happen. We need to check all of the agent binary store values from
		// the query for null to work out if this case exists.
		//
		// In theory just checking one of these values should be enough to
		// identify the condition but all three are done for safety and
		// correctness.
		if !unitRecord.SHA256.Valid ||
			!unitRecord.SHA384.Valid ||
			!unitRecord.Size.Valid {
			return nil, errors.Errorf(
				"unit %q has missing agent binaries in the model",
				unitRecord.UnitName,
			).Add(modelagenterrors.MissingAgentBinaries)
		}

		number, err := semversion.Parse(unitRecord.Version)
		if err != nil {
			return nil, errors.Errorf(
				"parsing unit %q version %q number: %w",
				unitRecord.UnitName, unitRecord.Version, err,
			)
		}

		rval[coreunit.Name(unitRecord.UnitName)] = coreagentbinary.Metadata{
			SHA256: unitRecord.SHA256.String,
			SHA384: unitRecord.SHA384.String,
			Size:   unitRecord.Size.Int64,
			Version: coreagentbinary.Version{
				Number: number,
				Arch:   unitRecord.Architecture,
			},
		}
	}

	return rval, nil
}

// GetUnitsNotAtTargetAgentVersion returns the list of units where
// their agent version is not the same as the models target agent version or
// who have no agent version reproted at all. If no units exist that match
// this criteria an empty slice is returned.
func (st *State) GetUnitsNotAtTargetAgentVersion(
	ctx context.Context,
) ([]coreunit.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT &unitName.*
FROM v_unit_target_agent_version
WHERE version != target_version
UNION
SELECT name
FROM unit
WHERE uuid NOT IN (SELECT unit_uuid
                   FROM v_unit_target_agent_version)
`

	queryStmt, err := st.Prepare(query, unitName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	rval := []unitName{}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&rval)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Errorf(
			"getting all unit names that are not at the target agent version: %w",
			err,
		)
	}

	names := make([]coreunit.Name, 0, len(rval))
	for _, uNameVal := range rval {
		names = append(names, coreunit.Name(uNameVal.Name))
	}
	return names, nil
}

// GetUnitRunningAgentBinaryVersion returns the running unit agent binary
// version for the given unit uuid.
// The following errors can be expected:
// - [applicationerrors.UnitNotFound] when the unit in question does not exist.
// - [modelagenterrors.AgentVersionNotFound] when no running agent version has
// been reported for the given machine.
func (st *State) GetUnitRunningAgentBinaryVersion(
	ctx context.Context,
	uuid coreunit.UUID,
) (coreagentbinary.Version, error) {
	db, err := st.DB()
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	info := unitAgentVersionInfo{}
	unitUUID := unitUUIDRef{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &unitAgentVersionInfo.*
FROM unit_agent_version AS uav
JOIN architecture AS a ON uav.architecture_id = a.id
WHERE uav.unit_uuid = $unitUUIDRef.unit_uuid
`, info, unitUUID)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking if unit %q exists: %w", uuid, err,
			)
		} else if !exists {
			return errors.Errorf(
				"unit %q does not exist", uuid,
			).Add(applicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, stmt, unitUUID).Get(&info)
		if errors.Is(err, sql.ErrNoRows) {
			return modelagenterrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf(
				"getting unit %q agent version: %w", uuid, err,
			)
		}
		return nil
	})
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	vers, err := semversion.Parse(info.Version)
	if err != nil {
		return coreagentbinary.Version{}, errors.Errorf(
			"parsing unit %q agent version %q: %w",
			uuid, info.Version, err,
		)
	}
	return coreagentbinary.Version{
		Number: vers,
		Arch:   info.ArchitectureName,
	}, nil
}

// GetUnitTargetAgentVersion returns the target agent version for the specified unit.
// The following error types can be expected:
// - [applicationerrors.UnitNotFound] when the unit does not exist.
// - [modelagenterrors.AgentVersionNotFound] when the agent version does not exist.
func (st *State) GetUnitTargetAgentVersion(ctx context.Context, uuid coreunit.UUID) (coreagentbinary.Version, error) {
	db, err := st.DB()
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	info := unitTargetAgentVersionInfo{}
	unitUUID := unitUUIDRef{
		UUID: uuid,
	}

	stmt, err := st.Prepare(`
SELECT &unitTargetAgentVersionInfo.*
FROM v_unit_target_agent_version
WHERE unit_uuid = $unitUUIDRef.unit_uuid
`, info, unitUUID)
	if err != nil {
		return coreagentbinary.Version{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		exists, err := st.checkUnitExists(ctx, tx, uuid)
		if err != nil {
			return errors.Errorf(
				"checking if unit %q exists: %w", uuid, err,
			)
		} else if !exists {
			return errors.Errorf(
				"unit %q does not exist", uuid,
			).Add(applicationerrors.UnitNotFound)
		}

		err = tx.Query(ctx, stmt, unitUUID).Get(&info)
		if errors.Is(err, sql.ErrNoRows) {
			return modelagenterrors.AgentVersionNotFound
		} else if err != nil {
			return errors.Errorf(
				"getting unit %q target agent version: %w",
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
			"parsing unit %q target agent version %q: %w",
			uuid, info.TargetVersion, err,
		)
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

	unitAgentVersion := unitAgentVersion{
		UnitUUID: uuid.String(),
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
		err := st.checkUnitNotDead(ctx, tx, uuid)
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
