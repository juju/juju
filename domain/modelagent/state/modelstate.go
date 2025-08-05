// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	"github.com/canonical/sqlair"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	corebase "github.com/juju/juju/core/base"
	"github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/semversion"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/modelagent"
	modelagenterrors "github.com/juju/juju/domain/modelagent/errors"
	"github.com/juju/juju/internal/errors"
)

// State is the means by which  the model agent accesses the model's state.
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
func (st *State) checkMachineNotDeadByName(
	ctx context.Context,
	tx *sqlair.TX,
	name string,
) error {
	ident := machineName{Name: name}
	stmt, err := st.Prepare(`
SELECT &machineLife.life_id FROM machine WHERE name = $machineName.name
`, machineLife{}, ident)
	if err != nil {
		return errors.Capture(err)
	}

	var machineLife machineLife
	err = tx.Query(ctx, stmt, ident).Get(&machineLife)
	if errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("machine %q does not exist", name).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return errors.Errorf(
			"checking if machine %q exists: %w",
			name, err,
		)
	}

	if machineLife.LifeID == domainlife.Dead {
		return errors.Errorf("machine %q is dead", name).Add(machineerrors.MachineIsDead)
	}

	return nil
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

// GetMachineCountNotUsingBase returns the number of machines that are not
// using one of the supplied bases. If no machines exist in the model or if
// no machines exist using a different base, zero is returned with no error. If
// a empty set of bases is provided every machine in the model will be included
// in the count.
func (st *State) GetMachineCountNotUsingBase(
	ctx context.Context,
	bases []corebase.Base,
) (int, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	machineBaseValues := make(machineBaseValues, 0, len(bases))
	for _, base := range bases {
		machineBaseValues = append(machineBaseValues, base.String())
	}
	machineCount := machineCount{}

	stmt, err := st.Prepare(`
WITH machine_bases AS (
    SELECT     CONCAT(os.name, '@', mp.channel) AS base
    FROM       machine AS m
    LEFT JOIN  machine_platform AS mp ON mp.machine_uuid = m.uuid
    LEFT JOIN  os ON mp.os_id = os.id
    WHERE      os.name IS NOT NULL AND os.name != ''
)
SELECT COUNT(*) AS &machineCount.count
FROM machine_bases AS mb
WHERE mb.base NOT IN ($machineBaseValues[:])
`,
		machineBaseValues, machineCount)
	if err != nil {
		return 0, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, machineBaseValues).Get(&machineCount)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return err
		}

		return nil
	})

	if err != nil {
		return 0, errors.Capture(err)
	}

	return machineCount.Count, nil
}

// GetMachineAgentBinaryMetadata reports the agent binary metadata that is
// currently running a given machine.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] when the machine being asked for does not
// exist.
// - [modelagenterrors.AgentVersionNotSet] when one or more machines in
// the model do not have their agent version set.
// - [modelagenterrors.MissingAgentBinaries] when the agent binaries don't exist
// for one or more machines in the model.
func (st *State) GetMachineAgentBinaryMetadata(ctx context.Context, mName string) (coreagentbinary.Metadata, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return coreagentbinary.Metadata{}, errors.Capture(err)
	}

	ident := machineName{Name: mName}
	stmt, err := st.Prepare(`
SELECT    mav.version AS &agentBinaryMetadata.version,
          mav.architecture_name AS &agentBinaryMetadata.architecture_name,
          osm.size AS &agentBinaryMetadata.size,
          osm.sha_256 AS &agentBinaryMetadata.sha_256,
          osm.sha_384 AS &agentBinaryMetadata.sha_384
FROM      v_machine_agent_version AS mav
LEFT JOIN v_agent_binary_store AS abs ON (
          mav.version = abs.version
AND       mav.architecture_id = abs.architecture_id)
LEFT JOIN object_store_metadata AS osm ON abs.object_store_uuid = osm.uuid
WHERE     mav.name = $machineName.name
`, agentBinaryMetadata{}, ident)
	if err != nil {
		return coreagentbinary.Metadata{}, errors.Capture(err)
	}

	var agentBinaryMetadata agentBinaryMetadata
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDeadByName(ctx, tx, mName)
		if err != nil {
			return errors.Errorf("checking machine %q exists: %w", mName, err)
		}

		err = tx.Query(ctx, stmt, ident).Get(&agentBinaryMetadata)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q not found", mName).Add(modelagenterrors.AgentVersionNotSet)
		} else if err != nil {
			return errors.Errorf("getting machine %q agent binary metadata: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return coreagentbinary.Metadata{}, errors.Capture(err)
	}

	if !agentBinaryMetadata.SHA256.Valid ||
		!agentBinaryMetadata.SHA384.Valid ||
		!agentBinaryMetadata.Size.Valid {
		return coreagentbinary.Metadata{}, errors.Errorf(
			"machine %q has missing agent binaries in the model",
			mName,
		).Add(modelagenterrors.MissingAgentBinaries)
	}

	number, err := semversion.Parse(agentBinaryMetadata.Version)
	if err != nil {
		return coreagentbinary.Metadata{}, errors.Errorf(
			"parsing machine %q agent binary version %q number: %w",
			mName, agentBinaryMetadata.Version, err,
		)
	}

	return coreagentbinary.Metadata{
		SHA256: agentBinaryMetadata.SHA256.String,
		SHA384: agentBinaryMetadata.SHA384.String,
		Size:   agentBinaryMetadata.Size.Int64,
		Version: coreagentbinary.Version{
			Number: number,
			Arch:   agentBinaryMetadata.Architecture,
		},
	}, nil
}

// GetMachinesAgentBinaryMetadata reports the agent binary metadata that each
// machine in the model is currently running. This is a bulk call to support
// operations such as model export where it is expected that the state of a
// model stays relatively static over the operation. This function will never
// provide enough granuality into what machine fails as part of the checks.
//
// The following errors can be expected:
// - [modelagenterrors.AgentVersionNotSet] when one or more machines in
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

	db, err := st.DB(ctx)
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
		).Add(modelagenterrors.AgentVersionNotSet)
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
// who have no agent version reported at all. If no machines exist that match
// this criteria an empty slice is returned.
func (st *State) GetMachinesNotAtTargetAgentVersion(
	ctx context.Context,
) ([]machine.Name, error) {
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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
// - [modelagenterrors.AgentVersionNotSet] when one or more units in
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

	db, err := st.DB(ctx)
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
		).Add(modelagenterrors.AgentVersionNotSet)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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
	db, err := st.DB(ctx)
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

// GetModelAgentStream gets the currently set agent stream for the model.
func (st *State) GetModelAgentStream(
	ctx context.Context,
) (modelagent.AgentStream, error) {
	// NOTE (tlm): This function is written on purpose to assume that an agent
	// version record has been established for the model. We assume that this is
	// always done on model creation.
	db, err := st.DB(ctx)
	if err != nil {
		return modelagent.AgentStream(-1), errors.Capture(err)
	}

	dbVal := agentVersionStream{}
	stmt, err := st.Prepare(`
SELECT &agentVersionStream.* FROM agent_version
`, dbVal)
	if err != nil {
		return modelagent.AgentStream(-1), errors.Capture(err)
	}

	rval := modelagent.AgentStream(-1)
	return rval, db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).Get(&dbVal)
		if errors.Is(err, sqlair.ErrNoRows) {
			// This should never happen but we write a specific error for the
			// case here to lead us back to the source if something ever goes
			// amiss.
			return errors.New(
				"agent version record has not been set for the model",
			)
		} else if err != nil {
			return errors.Errorf(
				"getting agent version stream for model: %w", err,
			)
		}

		rval = modelagent.AgentStream(dbVal.StreamID)
		return nil
	})
}

// GetModelTargetAgentVersion returns the agent version for the model.
// If the agent_version table has no data,
// [modelagenterrors.AgentVersionNotFound] is returned.
func (st *State) GetModelTargetAgentVersion(ctx context.Context) (semversion.Number, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return semversion.Zero, errors.Capture(err)
	}

	res := agentVersionTarget{}

	stmt, err := st.Prepare("SELECT &agentVersionTarget.target_version FROM agent_version", res)
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

	vers, err := semversion.Parse(res.TargetVersion)
	if err != nil {
		return semversion.Zero, errors.Errorf("parsing agent version: %w", err)
	}
	return vers, nil
}

// GetUnitUUIDByName returns the UUID for the named unit, returning an error
// satisfying [applicationerrors.UnitNotFound] if the unit doesn't exist.
func (st *State) GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error) {
	db, err := st.DB(ctx)
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

// IsControllerModel indicates if this model is running the Juju controller
// that owns this model. True is returned when this is the case.
func (s *State) IsControllerModel(ctx context.Context) (bool, error) {
	return false, errors.New("not implemented")
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
	db, err := st.DB(ctx)
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

// SetModelAgentStream is responsible for setting the agent stream that is in
// use by the current model.
func (st *State) SetModelAgentStream(
	ctx context.Context,
	agentStream modelagent.AgentStream,
) error {
	// NOTE (tlm): This function is written on purpose to ignore the fact that
	// if an agent version record does not exist in the database that this func
	// will blow up. This is on purpose as the caller should reasonably be able
	// to expect that this work has been done. It is not an error that the
	// caller can realisticly handle either so it makes no sense to have a
	// specific case for this. The agent_version table is a singleton table as
	// well.

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	streamSet := agentVersionStream{StreamID: int(agentStream)}
	stmt, err := st.Prepare(`
UPDATE agent_version SET stream_id = $agentVersionStream.stream_id
`, streamSet)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, streamSet).Run()
		if err != nil {
			return errors.Errorf(
				"setting model agent stream to %q: %w", agentStream, err,
			)
		}
		return nil
	})
}

// SetModelTargetAgentVersion is responsible for setting the current target
// agent version of the model. This function expects a precondition version to
// be supplied. The model's target agent version at the time the operation is
// applied must match the preCondition version or else an error is returned.
func (st *State) SetModelTargetAgentVersion(
	ctx context.Context,
	preCondition semversion.Number,
	toVersion semversion.Number,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	checkAgentVersionStmt, err := st.Prepare(`
SELECT &agentVersionTarget.*
FROM   agent_version
`,
		agentVersionTarget{})
	if err != nil {
		return errors.Capture(err)
	}

	toVersionInput := setAgentVersionTarget{TargetVersion: toVersion.String()}
	setAgentVersionStmt, err := st.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTarget.target_version
`,
		toVersionInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	preConditionVersionStr := preCondition.String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentAgentVersion := agentVersionTarget{}
		err := tx.Query(ctx, checkAgentVersionStmt).Get(&currentAgentVersion)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"checking current target agent version for model, no agent version has been previously set",
			)
		} else if err != nil {
			return errors.Errorf(
				"checking current target agent version for model: %w", err,
			)
		}

		if currentAgentVersion.TargetVersion != preConditionVersionStr {
			return errors.Errorf(
				"unable to set agent version for model. The agent version has changed to %q",
				currentAgentVersion.TargetVersion,
			)
		}

		// If the current version is the same as the toVersion we don't need to
		// perform the set operation. This avoids creating any churn in the
		// change log.
		if currentAgentVersion.TargetVersion == toVersionInput.TargetVersion {
			return nil
		}

		err = tx.Query(ctx, setAgentVersionStmt, toVersionInput).Run()
		if err != nil {
			return errors.Errorf(
				"setting target agent version to %q for model: %w",
				toVersion.String(), err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// SetModelTargetAgentVersionAndStream is responsible for setting the
// current target agent version of the model and the agent stream that is
// used. This function expects a precondition version to be supplied. The
// model's target version at the time the operation is applied must match
// the preCondition version or else an error is returned.
func (st *State) SetModelTargetAgentVersionAndStream(
	ctx context.Context,
	preCondition semversion.Number,
	toVersion semversion.Number,
	stream modelagent.AgentStream,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	checkAgentVersionStmt, err := st.Prepare(`
SELECT &agentVersionTarget.*
FROM   agent_version
`,
		agentVersionTarget{})
	if err != nil {
		return errors.Capture(err)
	}

	toVersionStreamInput := setAgentVersionTargetStream{
		StreamID:      int(stream),
		TargetVersion: toVersion.String(),
	}
	setAgentVersionStreamStmt, err := st.Prepare(`
UPDATE agent_version
SET    target_version = $setAgentVersionTargetStream.target_version,
       stream_id = $setAgentVersionTargetStream.stream_id
`,
		toVersionStreamInput,
	)
	if err != nil {
		return errors.Capture(err)
	}

	preConditionVersionStr := preCondition.String()
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		currentAgentVersion := agentVersionTarget{}
		err := tx.Query(ctx, checkAgentVersionStmt).Get(&currentAgentVersion)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.New(
				"checking current target agent version for model, no agent version has been previously set",
			)
		} else if err != nil {
			return errors.Errorf(
				"checking current target agent version for model: %w", err,
			)
		}

		if currentAgentVersion.TargetVersion != preConditionVersionStr {
			return errors.Errorf(
				"unable to set agent version and stream for model. The agent version has changed to %q",
				currentAgentVersion.TargetVersion,
			)
		}

		err = tx.Query(ctx, setAgentVersionStreamStmt, toVersionStreamInput).Run()
		if err != nil {
			return errors.Errorf(
				"setting target agent version and stream for model: %w", err,
			)
		}
		return nil
	})

	if err != nil {
		return errors.Capture(err)
	}

	return nil
}

// UpdateLatestAgentVersion persists the latest available agent version.
func (st *State) UpdateLatestAgentVersion(ctx context.Context, version semversion.Number) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	checkLatestAgentVersion, err := st.Prepare(`
SELECT &agentVersionInfo.*
FROM   agent_version
`, agentVersionInfo{})
	if err != nil {
		return errors.Capture(err)
	}

	modelAgentLatestVersion := latestAgentVersion{
		Version: version.String(),
	}
	stmt, err := st.Prepare(`
UPDATE agent_version
SET    latest_version = $latestAgentVersion.latest_version
`, modelAgentLatestVersion)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		agentVersionInfo := agentVersionInfo{}
		err := tx.Query(ctx, checkLatestAgentVersion).Get(&agentVersionInfo)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf(
				"agent version record has not been set for the model",
			)
		} else if err != nil {
			return errors.Errorf(
				"getting agent version: %w", err,
			)
		}

		currentTargetVersion, err := semversion.Parse(agentVersionInfo.TargetVersion)
		if err != nil {
			return errors.Errorf(
				"parsing target agent version: %w", err,
			)
		}
		if currentTargetVersion.Compare(version) == 1 {
			return errors.Errorf(
				"unable to update latest agent version to %q. The current agent version is %q", version, currentTargetVersion,
			).Add(modelagenterrors.LatestVersionDowngradeNotSupported)
		}

		currentLatestVersion, err := semversion.Parse(agentVersionInfo.LatestVersion)
		if err != nil {
			return errors.Errorf(
				"parsing latest agent version: %w", err,
			)
		}
		if res := currentLatestVersion.Compare(version); res == 1 {
			return errors.Errorf(
				"unable to update latest agent version to %q. The current latest agent version is %q", version, currentLatestVersion,
			).Add(modelagenterrors.LatestVersionDowngradeNotSupported)
		} else if res == 0 {
			// Nothing to do.
			return nil
		}

		return tx.Query(ctx, stmt, modelAgentLatestVersion).Run()
	})

	if err != nil {
		return errors.Errorf("updating latest agent version: %w", err)
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
	db, err := st.DB(ctx)
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
