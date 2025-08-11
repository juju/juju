// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"maps"
	"slices"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/collections/transform"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	"github.com/juju/juju/core/base"
	coredb "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/constraints"
	"github.com/juju/juju/domain/life"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	modelerrors "github.com/juju/juju/domain/model/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	clock  clock.Clock
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, clock clock.Clock, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		clock:     clock,
		logger:    logger,
	}
}

// AddMachine creates the net node and machines if required, depending
// on the placement.
// It returns the net node UUID for the machine and a list of child
// machine names that were created as part of the placement.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] if the parent machine (for container
// placement) does not exist.
func (st *State) AddMachine(ctx context.Context, args domainmachine.AddMachineArgs) (string, []machine.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	var (
		machineNames []machine.Name
		netNodeUUID  string
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		netNodeUUID, machineNames, err = PlaceMachine(ctx, tx, st, st.clock, args)
		return err
	})
	if err != nil {
		return "", nil, errors.Capture(err)
	}

	return netNodeUUID, machineNames, nil
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, mName machine.Name) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}
	machineUUIDParam := entityUUID{}
	queryMachine := `SELECT uuid AS &entityUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, machineUUIDParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for deleting machine row.
	deleteMachine := `DELETE FROM machine WHERE name = $machineName.name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineNameParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for deleting net node row.
	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE name = $machineName.name)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineNameParam)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&machineUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Errorf("looking up UUID for machine %q: %w", mName, err)
		}

		// Remove all basic machine data associated with the machine.
		if err := st.removeBasicMachineData(ctx, tx, machineUUIDParam); err != nil {
			return errors.Errorf("removing basic machine data for machine %q: %w", mName, err)
		}

		// Remove block devices for the machine.
		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Errorf("deleting block devices for machine %q: %w", mName, err)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineNameParam).Run(); err != nil {
			return errors.Errorf("deleting machine %q: %w", mName, err)
		}

		// Remove the net node for the machine.
		if err := tx.Query(ctx, deleteNodeStmt, machineNameParam).Run(); err != nil {
			return errors.Errorf("deleting net node for machine  %q: %w", mName, err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("deleting machine %q: %w", mName, err)
	}
	return nil
}

func (st *State) removeBasicMachineData(ctx context.Context, tx *sqlair.TX, machineUUID entityUUID) error {
	tables := []string{
		"machine_status",
		"machine_cloud_instance_status",
		"machine_cloud_instance",
		"machine_platform",
		"machine_agent_version",
		"machine_constraint",
		"machine_volume",
		"machine_filesystem",
		"machine_requires_reboot",
		"machine_lxd_profile",
		"machine_agent_presence",
		"machine_container_type",
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE machine_uuid = $entityUUID.uuid", table)
		stmt, err := st.Prepare(query, machineUUID)
		if err != nil {
			return errors.Errorf("preparing delete statement for %q: %w", table, err)
		}

		if err := tx.Query(ctx, stmt, machineUUID).Run(); err != nil {
			return errors.Errorf("deleting data from %q for machine %q: %w", table, machineUUID.UUID, err)
		}
	}
	return nil
}

// GetMachineLife returns the life status of the specified machine.
// It returns a MachineNotFound if the given machine doesn't exist.
func (st *State) GetMachineLife(ctx context.Context, mName machine.Name) (life.Life, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return -1, errors.Capture(err)
	}

	machineNameParam := machineName{Name: mName}
	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE name = $machineName.name`
	lifeStmt, err := st.Prepare(queryForLife, machineNameParam, machineLife{})
	if err != nil {
		return -1, errors.Capture(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, machineNameParam).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Errorf("looking up life for machine %q: %w", mName, err)
		}

		lifeResult = result.LifeID

		return nil
	})
	if err != nil {
		return -1, errors.Errorf("getting life status for machine %q: %w", mName, err)
	}
	return lifeResult, nil
}

// SetRunningAgentBinaryVersion sets the running agent binary version for the
// provided machine uuid. Any previously set values for this machine uuid will
// be overwritten by this call.
//
// The following errors can be expected:
// - [machineerrors.MachineNotFound] if the machine does not exist.
// - [machineerrors.MachineIsDead] if the machine is dead.
// - [coreerrors.NotSupported] if the architecture is not known to the database.
func (st *State) SetRunningAgentBinaryVersion(
	ctx context.Context,
	machineUUID string,
	version coreagentbinary.Version,
) error {
	db, err := st.DB(ctx)
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

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) IsMachineController(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT COUNT(*) AS &count.count
FROM   v_machine_is_controller
WHERE  machine_uuid = $entityUUID.uuid
`
	queryStmt, err := st.Prepare(query, entityUUID{}, result)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mUUID, err := st.getMachineUUIDFromName(ctx, tx, mName)
		if err != nil {
			return err
		}

		if err := tx.Query(ctx, queryStmt, mUUID).Get(&result); errors.Is(err, sqlair.ErrNoRows) {
			// If no rows are returned, the machine is not a controller.
			return nil
		} else if err != nil {
			return errors.Errorf("querying if machine %q is a controller: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Errorf("checking if machine %q is a controller: %w", mName, err)
	}

	return result.Count == 1, nil
}

// IsMachineManuallyProvisioned returns whether the machine is a manual machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) IsMachineManuallyProvisioned(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT     COUNT(m.uuid) AS &count.count
FROM       machine AS m
JOIN  machine_manual AS mm ON m.uuid = mm.machine_uuid
WHERE      m.uuid = $entityUUID.uuid
`
	queryStmt, err := st.Prepare(query, entityUUID{}, count{})
	if err != nil {
		return false, errors.Capture(err)
	}

	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		mUUID, err := st.getMachineUUIDFromName(ctx, tx, mName)
		if err != nil {
			return err
		}

		if err := tx.Query(ctx, queryStmt, mUUID).Get(&result); err != nil {
			return errors.Errorf("querying if machine %q is a controller: %w", mName, err)
		}

		return nil
	}); err != nil {
		return false, errors.Errorf("checking if machine %q is manual: %w", mName, err)
	}

	return result.Count == 1, nil
}

func (st *State) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, mName machine.Name) (entityUUID, error) {
	machineNameParam := machineName{Name: mName}
	machineUUIDOutput := entityUUID{}
	query := `SELECT uuid AS &entityUUID.uuid FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, machineUUIDOutput)
	if err != nil {
		return entityUUID{}, errors.Capture(err)
	}

	if err := tx.Query(ctx, queryStmt, machineNameParam).Get(&machineUUIDOutput); errors.Is(err, sqlair.ErrNoRows) {
		return entityUUID{}, errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return entityUUID{}, errors.Errorf("querying UUID for machine %q: %w", mName, err)
	}
	return machineUUIDOutput, nil
}

// AllMachineNames retrieves the names of all machines in the model.
func (st *State) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `SELECT name AS &machineName.* FROM machine`
	queryStmt, err := st.Prepare(query, machineName{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []machineName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying all machines: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Transform the results ([]machineName) into a slice of machine.Name.
	machineNames := transform.Slice(results, machineName.nameSliceTransform)

	return machineNames, nil
}

// GetMachineParentUUID returns the parent UUID of the specified machine.
// It returns a MachineNotFound if the machine does not exist.
// It returns a MachineHasNoParent if the machine has no parent.
func (st *State) GetMachineParentUUID(ctx context.Context, uuid string) (machine.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Prepare query for checking that the machine exists.
	currentMachineUUID := entityUUID{UUID: uuid}
	query := `SELECT uuid AS &entityUUID.uuid FROM machine WHERE uuid = $entityUUID.uuid`
	queryStmt, err := st.Prepare(query, currentMachineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Prepare query for parent UUID.
	var parentUUID machine.UUID
	parentUUIDParam := machineParent{}
	parentQuery := `
SELECT parent_uuid AS &machineParent.parent_uuid
FROM machine_parent WHERE machine_uuid = $entityUUID.uuid`
	parentQueryStmt, err := st.Prepare(parentQuery, currentMachineUUID, parentUUIDParam)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		outUUID := entityUUID{} // This value doesn't really matter, it is just a way to check existence
		err := tx.Query(ctx, queryStmt, currentMachineUUID).Get(&outUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", uuid, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("checking existence of machine %q: %w", uuid, err)
		}

		// Query for the parent UUID.
		err = tx.Query(ctx, parentQueryStmt, currentMachineUUID).Get(&parentUUIDParam)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", uuid, machineerrors.MachineHasNoParent)
		}
		if err != nil {
			return errors.Errorf("querying parent UUID for machine %q: %w", uuid, err)
		}

		parentUUID = machine.UUID(parentUUIDParam.ParentUUID)

		return nil
	})
	if err != nil {
		return parentUUID, errors.Errorf("getting parent UUID for machine %q: %w", uuid, err)
	}
	return parentUUID, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (st *State) GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid entityUUID
	currentMachineName := machineName{Name: name}
	query := `SELECT uuid AS &entityUUID.uuid FROM machine WHERE name = $machineName.name`
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
		return machine.UUID(uuid.UUID), errors.Errorf("getting UUID for machine %q: %w", name, err)
	}
	return machine.UUID(uuid.UUID), nil
}

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
func (st *State) ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return false, errors.Capture(err)
	}

	machineNameParam := machineName{Name: mName}
	result := keepInstance{}
	query := `
SELECT &keepInstance.keep_instance
FROM   machine
WHERE  name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, result)
	if err != nil {
		return false, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Errorf("querying machine %q keep instance: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return false, errors.Errorf("check for machine %q keep instance: %w", mName, err)
	}

	return result.KeepInstance, nil
}

// SetKeepInstance sets whether the machine cloud instance will be retained
// when the machine is removed from Juju. This is only relevant if an instance
// exists.
func (st *State) SetKeepInstance(ctx context.Context, mName machine.Name, keep bool) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine uuid.
	machineUUID := entityUUID{}
	machineNameParam := machineName{Name: mName}
	machineExistsQuery := `
SELECT uuid AS &entityUUID.uuid
FROM   machine
WHERE  name = $machineName.name`
	machineExistsStmt, err := st.Prepare(machineExistsQuery, machineUUID, machineNameParam)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for updating machine keep instance.
	keepInstanceParam := keepInstance{KeepInstance: keep}
	keepInstanceQuery := `
UPDATE machine
SET    keep_instance = $keepInstance.keep_instance
WHERE  name = $machineName.name`
	keepInstanceStmt, err := st.Prepare(keepInstanceQuery, keepInstanceParam, machineNameParam)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid before attempting to update it,
		// and return an error if it doesn't.
		err := tx.Query(ctx, machineExistsStmt, machineNameParam).Get(&machineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		// Update machine keep instance.
		err = tx.Query(ctx, keepInstanceStmt, keepInstanceParam, machineNameParam).Run()
		if err != nil {
			return errors.Errorf("setting keep instance for machine %q: %w", mName, err)
		}
		return nil
	})
}

// AppliedLXDProfileNames returns the names of the LXD profiles on the machine.
func (st *State) AppliedLXDProfileNames(ctx context.Context, mUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	instanceDataQuery := instanceData{MachineUUID: mUUID}
	isProvisionedStmt, err := st.Prepare(`
SELECT   &instanceData.instance_id
FROM     machine_cloud_instance
WHERE    machine_uuid = $instanceData.machine_uuid`, instanceDataQuery)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lxdProfileQuery := lxdProfile{MachineUUID: mUUID}
	queryStmt, err := st.Prepare(`
SELECT   &lxdProfile.name
FROM     machine_lxd_profile
WHERE    machine_uuid = $lxdProfile.machine_uuid
ORDER BY array_index ASC`, lxdProfileQuery)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var result []lxdProfile
	if err := db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var instanceData instanceData
		err := tx.Query(ctx, isProvisionedStmt, instanceDataQuery).Get(&instanceData)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking machine cloud instance for machine %q: %w", mUUID, err)
		}

		// If the machine is not provisioned, return an error.
		instanceID := instanceData.InstanceID
		if !instanceID.Valid || instanceID.V == "" {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		}

		err = tx.Query(ctx, queryStmt, lxdProfileQuery).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving lxd profiles for machine %q: %w", mUUID, err)
		}
		return nil
	}); err != nil {
		return nil, err
	}

	lxdProfiles := make([]string, len(result))
	for i, lxdProfile := range result {
		lxdProfiles[i] = lxdProfile.Name
	}
	return lxdProfiles, nil
}

// SetAppliedLXDProfileNames sets the list of LXD profile names to the
// lxd_profile table for the given machine. This method will overwrite the list
// of profiles for the given machine without any checks.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (st *State) SetAppliedLXDProfileNames(ctx context.Context, mUUID string, profileNames []string) error {
	if len(profileNames) == 0 {
		return nil
	}

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Errorf("cannot get database to set lxd profiles %v for machine %q: %w", profileNames, mUUID, err)
	}

	queryMachineUUID := entityUUID{UUID: mUUID}
	checkMachineExistsStmt, err := st.Prepare(`
SELECT uuid AS &entityUUID.uuid
FROM   machine
WHERE  machine.uuid = $entityUUID.uuid`, queryMachineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	instanceDataQuery := instanceData{MachineUUID: mUUID}
	isProvisionedStmt, err := st.Prepare(`
SELECT   &instanceData.instance_id
FROM     machine_cloud_instance
WHERE    machine_uuid = $instanceData.machine_uuid`, instanceDataQuery)
	if err != nil {
		return errors.Capture(err)
	}

	queryLXDProfile := lxdProfile{MachineUUID: mUUID}
	retrievePreviousProfilesStmt, err := st.Prepare(`
SELECT   name AS &lxdProfile.name
FROM     machine_lxd_profile
WHERE    machine_uuid = $lxdProfile.machine_uuid
ORDER BY array_index ASC`, queryLXDProfile)
	if err != nil {
		return errors.Capture(err)
	}

	removePreviousProfilesStmt, err := st.Prepare(
		`DELETE FROM machine_lxd_profile WHERE machine_uuid = $entityUUID.uuid`,
		entityUUID{},
	)
	if err != nil {
		return errors.Capture(err)
	}

	setLXDProfileStmt, err := st.Prepare(`
INSERT INTO machine_lxd_profile (*)
VALUES      ($lxdProfile.*)`, lxdProfile{})
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var machineExists entityUUID
		err = tx.Query(ctx, checkMachineExistsStmt, queryMachineUUID).Get(&machineExists)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Errorf("checking if machine %q exists: %w", mUUID, err)
		}

		// Check if the machine is provisioned
		var instanceData instanceData
		err = tx.Query(ctx, isProvisionedStmt, instanceDataQuery).Get(&instanceData)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		} else if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("checking machine cloud instance for machine %q: %w", mUUID, err)
		}

		// If the machine is not provisioned, return an error.
		instanceID := instanceData.InstanceID
		if !instanceID.Valid || instanceID.V == "" {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.NotProvisioned)
		}

		// Retrieve the existing profiles to check if the input is the exactly
		// the same as the existing ones and in that case no insert is needed.
		var existingProfiles []lxdProfile
		err = tx.Query(ctx, retrievePreviousProfilesStmt, queryLXDProfile).GetAll(&existingProfiles)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("retrieving previous profiles for machine %q: %w", mUUID, err)
		}
		// Compare the input with the existing profiles.
		somethingToInsert := len(existingProfiles) != len(profileNames)
		// Only compare the order of the profiles if the input size is the same
		// as the existing profiles.
		if !somethingToInsert {
			for i, profile := range existingProfiles {
				if profileNames[i] != profile.Name {
					somethingToInsert = true
					fmt.Printf("breaking, profile: %v\n", profile)
					break
				}
			}
		}
		// No error to return and nothing to insert.
		if !somethingToInsert {
			fmt.Printf("existing profiles: %v\n", existingProfiles)
			return nil
		}

		// Make sure to clean up any existing profiles on the given machine.
		if err := tx.Query(ctx, removePreviousProfilesStmt, queryMachineUUID).Run(); err != nil {
			return errors.Errorf("remove previous profiles for machine %q: %w", mUUID, err)
		}

		profilesToInsert := make([]lxdProfile, 0, len(profileNames))
		// Insert the profiles in the order they are provided.
		for index, profileName := range profileNames {
			profilesToInsert = append(profilesToInsert,
				lxdProfile{
					Name:        profileName,
					MachineUUID: mUUID,
					Index:       index,
				},
			)
		}
		if err := tx.Query(ctx, setLXDProfileStmt, profilesToInsert).Run(); err != nil {
			return errors.Errorf("setting lxd profiles %v for machine %q: %w", profileNames, mUUID, err)
		}
		return nil
	})
}

// GetNamesForUUIDs returns a map of machine UUIDs to machine Names based
// on the given machine UUIDs.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (st *State) GetNamesForUUIDs(ctx context.Context, machineUUIDs []string) (map[machine.UUID]machine.Name, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Errorf("cannot get database find names for machines %q: %w", machineUUIDs, err)
	}

	type nameAndUUID availabilityZoneName
	type mUUIDs []string
	uuids := mUUIDs(machineUUIDs)

	stmt, err := st.Prepare(`
SELECT &nameAndUUID.*
FROM   machine
WHERE  machine.uuid IN ($mUUIDs[:])`, nameAndUUID{}, uuids)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var namesAndUUIDs []nameAndUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, uuids).GetAll(&namesAndUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		return nil
	})

	return transform.SliceToMap(namesAndUUIDs, func(n nameAndUUID) (machine.UUID, machine.Name) {
		return machine.UUID(n.UUID), machine.Name(n.Name)
	}), err
}

// GetMachineArchesForApplication returns a map of machine names to their
// instance IDs. This will ignore non-provisioned machines or container
// machines.
func (st *State) GetAllProvisionedMachineInstanceID(ctx context.Context) (map[machine.Name]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	query := `
SELECT    m.name AS &machineInstance.machine_name,
          mci.instance_id AS &machineInstance.instance_id
FROM      machine AS m
JOIN      machine_cloud_instance AS mci ON m.uuid = mci.machine_uuid
WHERE     mci.instance_id IS NOT NULL AND mci.instance_id != ''
AND       (
            SELECT COUNT(mp.machine_uuid) AS cc
            FROM machine_parent AS mp
            WHERE mp.machine_uuid = m.uuid
          ) = 0
`
	stmt, err := st.Prepare(query, machineInstance{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []machineInstance
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt).GetAll(&results)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil {
			return errors.Errorf("querying all provisioned machines: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	instanceIDs := make(map[machine.Name]string)
	for _, result := range results {
		if result.IsContainer > 0 || result.MachineName == "" || result.InstanceID == "" {
			continue
		}
		instanceIDs[machine.Name(result.MachineName)] = result.InstanceID
	}
	return instanceIDs, nil
}

// SetMachineHostname sets the hostname for the given machine.
// Also updates the agent_started_at timestamp.
func (st *State) SetMachineHostname(ctx context.Context, mUUID string, hostname string) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	currentMachineUUID := entityUUID{UUID: mUUID}
	query := `SELECT uuid AS &entityUUID.uuid FROM machine WHERE uuid = $entityUUID.uuid`
	queryStmt, err := st.Prepare(query, currentMachineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	var nullableHostname sql.Null[string]
	if hostname != "" {
		nullableHostname.Valid = true
		nullableHostname.V = hostname
	}

	currentMachineHostName := machineHostName{
		Hostname:       nullableHostname,
		AgentStartedAt: st.clock.Now(),
	}
	updateQuery := `
UPDATE machine
SET    hostname = $machineHostName.hostname, agent_started_at = $machineHostName.agent_started_at
WHERE  uuid = $entityUUID.uuid`
	updateStmt, err := st.Prepare(updateQuery, currentMachineUUID, currentMachineHostName)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		err := tx.Query(ctx, queryStmt, currentMachineUUID).Get(&currentMachineUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying UUID for machine %q: %w", mUUID, err)
		}

		// Update the hostname.
		err = tx.Query(ctx, updateStmt, currentMachineUUID, currentMachineHostName).Run()
		if err != nil {
			return errors.Errorf("updating hostname for machine %q: %w", mUUID, err)
		}
		return nil
	})
}

// GetSupportedContainersTypes returns the supported container types for the
// given machine.
func (st *State) GetSupportedContainersTypes(ctx context.Context, mUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	currentMachineUUID := entityUUID{UUID: mUUID}
	query := `
SELECT ct.value AS &containerType.container_type
FROM machine AS m
LEFT JOIN machine_container_type AS mct ON m.uuid = mct.machine_uuid
LEFT JOIN container_type AS ct ON mct.container_type_id = ct.id
WHERE uuid = $entityUUID.uuid
`
	queryStmt, err := st.Prepare(query, currentMachineUUID, containerType{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var containerTypes []containerType
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, currentMachineUUID).GetAll(&containerTypes)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mUUID, machineerrors.MachineNotFound)
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("getting supported container types %q: %w", mUUID, err)
	}

	result := make([]string, len(containerTypes))
	for i, ct := range containerTypes {
		result[i] = ct.ContainerType
	}
	return result, nil
}

// GetMachineContainers returns the names of the machines which have as parent
// the specified machine.
func (st *State) GetMachineContainers(ctx context.Context, mUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	ident := entityUUID{UUID: mUUID}
	query := `
SELECT &machineName.*
FROM machine
JOIN machine_parent ON machine.uuid = machine_parent.machine_uuid
WHERE parent_uuid = $entityUUID.uuid`
	queryStmt, err := st.Prepare(query, machineName{}, ident)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []machineName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := st.checkMachineNotDead(ctx, tx, mUUID)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, queryStmt, ident).GetAll(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	return transform.Slice(results, func(v machineName) string {
		return v.Name.String()
	}), nil
}

// GetMachinePrincipalApplications returns the names of the principal
// (non-subordinate) applications for the specified machine.
func (st *State) GetMachinePrincipalApplications(ctx context.Context, mName machine.Name) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	principalQuery := `
SELECT a.name AS &entityName.name
FROM machine AS m
JOIN net_node AS nn ON m.net_node_uuid = nn.uuid
LEFT JOIN unit AS u ON u.net_node_uuid = nn.uuid
LEFT JOIN application AS a ON u.application_uuid = a.uuid
LEFT JOIN charm AS c ON a.charm_uuid = c.uuid
LEFT JOIN charm_metadata AS cm ON cm.charm_uuid = c.uuid
WHERE m.uuid = $entityUUID.uuid AND cm.subordinate = FALSE
ORDER BY a.name ASC
`
	principalQueryStmt, err := st.Prepare(principalQuery, entityUUID{}, entityName{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	var appNames []entityName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, err := st.getMachineUUIDFromName(ctx, tx, mName)
		if err != nil {
			return err
		}

		err = tx.Query(ctx, principalQueryStmt, machineUUID).GetAll(&appNames)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, errors.Errorf("querying principal units for machine %q: %w", mName, err)
	}
	result := make([]string, len(appNames))
	for i, unit := range appNames {
		result[i] = unit.Name
	}
	return result, nil
}

// GetMachinePlacement returns the placement structure as it was recorded for
// the given machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (st *State) GetMachinePlacementDirective(ctx context.Context, mName string) (*string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &placementDirective.*
FROM machine_placement
WHERE machine_uuid = $entityUUID.uuid
`, placementDirective{}, entityUUID{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var placementDirective placementDirective
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, err := st.getMachineUUIDFromName(ctx, tx, machine.Name(mName))
		if err != nil {
			return err
		}
		err = tx.Query(ctx, stmt, machineUUID).Get(&placementDirective)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return errors.Errorf("querying placement for machine %q: %w", mName, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}

	if placementDirective.Directive.Valid {
		return &placementDirective.Directive.V, nil
	}
	return nil, nil
}

// GetMachineConstraints returns the constraints for the given machine.
// Empty constraints are returned if no constraints exist for the given
// machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (st *State) GetMachineConstraints(ctx context.Context, mName string) (constraints.Constraints, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &machineConstraint.*
FROM v_machine_constraint
WHERE machine_uuid = $entityUUID.uuid;
`, machineConstraint{}, entityUUID{})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}
	var result machineConstraints
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, err := st.getMachineUUIDFromName(ctx, tx, machine.Name(mName))
		if err != nil {
			return err
		}
		err = tx.Query(ctx, stmt, machineUUID).GetAll(&result)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		return nil
	})
	if err != nil {
		return constraints.Constraints{}, errors.Errorf("getting constraints for machine %q: %w", mName, err)
	}

	return decodeConstraints(result), nil
}

// GetModelConstraints returns the currently set constraints for the model.
// The following error types can be expected:
// - [modelerrors.NotFound]: when no model exists to set constraints for.
// - [modelerrors.ConstraintsNotFound]: when no model constraints have been
// set for the model.
// Note: This method should mirror the model domain method of the same name.
func (st *State) GetModelConstraints(
	ctx context.Context,
) (constraints.Constraints, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectTagStmt, err := st.Prepare(
		"SELECT &dbConstraintTag.* FROM v_model_constraint_tag", dbConstraintTag{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectSpaceStmt, err := st.Prepare(
		"SELECT &dbConstraintSpace.* FROM v_model_constraint_space", dbConstraintSpace{},
	)
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	selectZoneStmt, err := st.Prepare(
		"SELECT &dbConstraintZone.* FROM v_model_constraint_zone", dbConstraintZone{})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	var (
		cons   dbConstraint
		tags   []dbConstraintTag
		spaces []dbConstraintSpace
		zones  []dbConstraintZone
	)
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := modelExists(ctx, st, tx); err != nil {
			return errors.Capture(err)
		}

		cons, err = st.getModelConstraints(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}
		err = tx.Query(ctx, selectTagStmt).GetAll(&tags)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint tags: %w", err)
		}
		err = tx.Query(ctx, selectSpaceStmt).GetAll(&spaces)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint spaces: %w", err)
		}
		err = tx.Query(ctx, selectZoneStmt).GetAll(&zones)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting constraint zones: %w", err)
		}
		return nil
	})
	if err != nil {
		return constraints.Constraints{}, errors.Capture(err)
	}

	return cons.toValue(tags, spaces, zones)
}

// getModelConstraints returns the values set in the constraints table for the
// current model. If no constraints are currently set
// for the model an error satisfying [modelerrors.ConstraintsNotFound] will be
// returned.
func (st *State) getModelConstraints(
	ctx context.Context,
	tx *sqlair.TX,
) (dbConstraint, error) {
	var constraint dbConstraint

	stmt, err := st.Prepare("SELECT &dbConstraint.* FROM v_model_constraint", constraint)
	if err != nil {
		return dbConstraint{}, errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&constraint)
	if errors.Is(err, sql.ErrNoRows) {
		return dbConstraint{}, errors.New(
			"no constraints set for model",
		).Add(modelerrors.ConstraintsNotFound)
	}
	if err != nil {
		return dbConstraint{}, errors.Errorf("getting model constraints: %w", err)
	}
	return constraint, nil
}

func modelExists(ctx context.Context, preparer domain.Preparer, tx *sqlair.TX) error {
	var modelUUID entityUUID
	stmt, err := preparer.Prepare(`SELECT &entityUUID.uuid FROM model;`, modelUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = tx.Query(ctx, stmt).Get(&modelUUID)
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("model does not exist").Add(modelerrors.NotFound)
	}
	if err != nil {
		return errors.Errorf("checking model if model exists: %w", err)
	}

	return nil
}

// GetMachineBase returns the base for the given machine.
// Since the machine_platform table is populated when creating a machine, there
// should always be a base for a machine.
//
// The following errors may be returned:
// - [machineerrors.MachineNotFound] if the machine does not exist.
func (st *State) GetMachineBase(ctx context.Context, mName string) (base.Base, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return base.Base{}, errors.Capture(err)
	}

	stmt, err := st.Prepare(`
SELECT &machinePlatform.*
FROM v_machine_platform
WHERE machine_uuid = $entityUUID.uuid
`, machinePlatform{}, entityUUID{})
	if err != nil {
		return base.Base{}, errors.Capture(err)
	}

	var result machinePlatform
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machineUUID, err := st.getMachineUUIDFromName(ctx, tx, machine.Name(mName))
		if err != nil {
			return err
		}
		return tx.Query(ctx, stmt, machineUUID).Get(&result)
	})
	if err != nil {
		return base.Base{}, errors.Errorf("querying machine base for machine %q: %w", mName, err)
	}

	return base.ParseBase(result.OSName, result.Channel)
}

// CountMachinesInSpace counts the number of machines with address in a given
// space. This method counts the distinct occurrences of net nodes of the
// addresses, meaning that if a machine has multiple addresses in the same
// subnet it will be counted only once.
func (st *State) CountMachinesInSpace(ctx context.Context, spUUID string) (int64, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return 0, errors.Capture(err)
	}

	ident := entityUUID{UUID: spUUID}
	stmt, err := st.Prepare(`
SELECT COUNT(DISTINCT net_node_uuid) AS &count.count
FROM ip_address AS ip
JOIN v_space_subnet AS sp ON ip.subnet_uuid = sp.subnet_uuid
WHERE sp.uuid = $entityUUID.uuid
`, count{}, ident)
	if err != nil {
		return 0, errors.Capture(err)
	}

	var count count
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, stmt, ident).Get(&count)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("space %s", spUUID).Add(networkerrors.SpaceNotFound)
		}
		return nil
	})
	if err != nil {
		return 0, errors.Errorf("counting machines in space %q: %w", spUUID, err)
	}

	return count.Count, nil
}

// GetSSHHostKeys returns the SSH host keys for the given machine.
func (st *State) GetSSHHostKeys(ctx context.Context, mUUID string) ([]string, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}

	query := `SELECT &sshHostKey.*
FROM machine_ssh_host_key
WHERE machine_uuid = $entityUUID.uuid`
	queryStmt, err := st.Prepare(query, sshHostKey{}, machineUUID)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var sshHostKeys []sshHostKey
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the machine exists.
		if err := st.checkMachineNotDead(ctx, tx, mUUID); err != nil {
			return errors.Capture(err)
		}

		err = tx.Query(ctx, queryStmt, machineUUID).GetAll(&sshHostKeys)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Errorf("querying SSH host keys for machine %q: %w", mUUID, err)
		}
		return nil
	})
	if err != nil {
		return nil, errors.Capture(err)
	}
	// Convert the SSH host keys to a slice of strings.
	result := make([]string, len(sshHostKeys))
	for i, key := range sshHostKeys {
		result[i] = key.Key
	}
	return result, nil
}

// SetSSHHostKeys sets the SSH host keys for the given machine.
// This will overwrite the existing SSH host keys for the machine.
// [machineerrors.MachineNotFound] will be returned if the machine does not
// exist.
func (st *State) SetSSHHostKeys(ctx context.Context, mUUID string, sshHostKeys []string) error {
	if len(sshHostKeys) == 0 {
		return nil // Nothing to do, no SSH host keys to set.
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	machineUUID := entityUUID{UUID: mUUID}

	removePreviousKeysStmt, err := st.Prepare(`
DELETE FROM machine_ssh_host_key
WHERE  machine_uuid = $entityUUID.uuid`, machineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	setSSHHostKeyStmt, err := st.Prepare(`
INSERT INTO machine_ssh_host_key (*)
VALUES      ($sshHostKey.*)`, sshHostKey{})
	if err != nil {
		return errors.Capture(err)
	}

	keysToInsert := make([]sshHostKey, len(sshHostKeys))
	for i, key := range sshHostKeys {
		uuid, err := uuid.NewUUID()
		if err != nil {
			return errors.Errorf("generating UUID for SSH host key %d: %w", i, err)
		}

		keysToInsert[i] = sshHostKey{
			UUID:        uuid.String(),
			MachineUUID: mUUID,
			Key:         key,
		}
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Check if the machine exists.
		if err := st.checkMachineNotDead(ctx, tx, mUUID); err != nil {
			return errors.Capture(err)
		}

		// Remove any previous SSH host keys for the machine.
		if err := tx.Query(ctx, removePreviousKeysStmt, machineUUID).Run(); err != nil {
			return errors.Errorf("removing previous SSH host keys for machine %q: %w", mUUID, err)
		}

		// Insert the new SSH host keys.
		if err := tx.Query(ctx, setSSHHostKeyStmt, keysToInsert).Run(); err != nil {
			return errors.Errorf("setting SSH host keys %v for machine %q: %w", sshHostKeys, mUUID, err)
		}
		return nil
	})
}

// NamespaceForWatchMachineCloudInstance returns the namespace for watching
// machine cloud instance changes.
func (*State) NamespaceForWatchMachineCloudInstance() string {
	return "machine_cloud_instance"
}

// NamespaceForWatchMachineLXDProfiles returns the namespace for watching
// machine LXD profile changes.
func (*State) NamespaceForWatchMachineLXDProfiles() string {
	return "machine_lxd_profile"
}

// NamespaceForWatchMachineReboot returns the namespace string used for
// tracking machine reboot events in the model.
func (*State) NamespaceForWatchMachineReboot() string {
	return "machine_requires_reboot"
}

// NamespaceForMachineLife returns the namespace string used for
// tracking machine lifecycle changes in the model.
func (*State) NamespaceForMachineLife() string {
	return "custom_machine_name_lifecycle"
}

// NamespaceForMachineAndMachineUnitLife returns the namespace string used
// for tracking machine and machine unit lifecycle events in the model.
func (*State) NamespaceForMachineAndMachineUnitLife() (string, string) {
	return "custom_machine_name_lifecycle",
		"custom_machine_unit_name_lifecycle"
}

// InitialWatchModelMachinesStatement returns the table and the initial watch
// statement for watching life changes of non-container machines.
func (*State) InitialWatchModelMachinesStatement() (string, string) {
	return "custom_machine_name_lifecycle", "SELECT name FROM machine WHERE name NOT LIKE '%/%'"
}

// InitialWatchModelMachineLifeAndStartTimesStatement returns the namespace and the initial watch
// statement for watching life and agent start time changes machines.
func (*State) InitialWatchModelMachineLifeAndStartTimesStatement() (string, string) {
	return "custom_machine_lifecycle_start_time", "SELECT name FROM machine"
}

// InitialMachineContainerLifeStatement returns the table and the initial watch
// statement for watching life changes of container machines.
func (*State) InitialMachineContainerLifeStatement() (string, string, func(string) string) {
	return "custom_machine_name_lifecycle", "SELECT name FROM machine WHERE name LIKE ?", func(prefix string) string {
		return prefix + "%"
	}
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (*State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT name FROM machine"
}

func encodeLife(v life.Life) (int64, error) {
	// Encode the life status as an int64.
	// This is a simple mapping, but can be extended if needed.
	switch v {
	case life.Alive:
		return 0, nil
	case life.Dying:
		return 1, nil
	case life.Dead:
		return 2, nil
	default:
		return 0, errors.Errorf("encoding life status: %v", v)
	}
}

// decodeConstraints flattens and maps the list of rows of machine_constraint
// to get a single constraints.Constraints. The flattening is needed because of
// the spaces, tags and zones constraints which are slices. We can safely assume
// that the non-slice values are repeated on every row so we can safely
// overwrite the previous value on each iteration.
func decodeConstraints(cons machineConstraints) constraints.Constraints {
	var res constraints.Constraints

	// Empty constraints is not an error case, so return early the empty
	// result.
	if len(cons) == 0 {
		return res
	}

	// Unique spaces, tags and zones:
	spaces := make(map[string]constraints.SpaceConstraint)
	tags := set.NewStrings()
	zones := set.NewStrings()

	for _, row := range cons {
		if row.Arch.Valid {
			res.Arch = &row.Arch.String
		}
		if row.CPUCores.Valid {
			cpuCores := uint64(row.CPUCores.V)
			res.CpuCores = &cpuCores
		}
		if row.CPUPower.Valid {
			cpuPower := uint64(row.CPUPower.V)
			res.CpuPower = &cpuPower
		}
		if row.Mem.Valid {
			mem := uint64(row.Mem.V)
			res.Mem = &mem
		}
		if row.RootDisk.Valid {
			rootDisk := uint64(row.RootDisk.V)
			res.RootDisk = &rootDisk
		}
		if row.RootDiskSource.Valid {
			res.RootDiskSource = &row.RootDiskSource.String
		}
		if row.InstanceRole.Valid {
			res.InstanceRole = &row.InstanceRole.String
		}
		if row.InstanceType.Valid {
			res.InstanceType = &row.InstanceType.String
		}
		if row.ContainerType.Valid {
			containerType := instance.ContainerType(row.ContainerType.String)
			res.Container = &containerType
		}
		if row.VirtType.Valid {
			res.VirtType = &row.VirtType.String
		}
		if row.AllocatePublicIP.Valid {
			res.AllocatePublicIP = &row.AllocatePublicIP.Bool
		}
		if row.ImageID.Valid {
			res.ImageID = &row.ImageID.String
		}
		if row.SpaceName.Valid {
			var exclude bool
			if row.SpaceExclude.Valid {
				exclude = row.SpaceExclude.Bool
			}
			spaces[row.SpaceName.String] = constraints.SpaceConstraint{
				SpaceName: row.SpaceName.String,
				Exclude:   exclude,
			}
		}
		if row.Tag.Valid {
			tags.Add(row.Tag.String)
		}
		if row.Zone.Valid {
			zones.Add(row.Zone.String)
		}
	}

	// Add the unique spaces, tags and zones to the result:
	if len(spaces) > 0 {
		res.Spaces = ptr(slices.Collect(maps.Values(spaces)))
	}
	if len(tags) > 0 {
		tagsSlice := tags.SortedValues()
		res.Tags = &tagsSlice
	}
	if len(zones) > 0 {
		zonesSlice := zones.SortedValues()
		res.Zones = &zonesSlice
	}

	return res
}

func ptr[T any](v T) *T {
	return &v
}
