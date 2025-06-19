// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/canonical/sqlair"
	"github.com/juju/clock"
	"github.com/juju/collections/transform"

	coreagentbinary "github.com/juju/juju/core/agentbinary"
	coredb "github.com/juju/juju/core/database"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
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

// CreateMachine creates or updates the specified machine.
// Adds a row to machine table, as well as a row to the net_node table.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) CreateMachine(ctx context.Context, machineName machine.Name, nodeUUID string, machineUUID machine.UUID, nonce *string) error {
	return st.createMachine(ctx, createMachineArgs{
		name:        machineName,
		netNodeUUID: nodeUUID,
		machineUUID: machineUUID,
		nonce:       nonce,
	})
}

// CreateMachineWithParent creates or updates the specified machine with a
// parent.
// Adds a row to machine table, as well as a row to the net_node table, and adds
// a row to the machine_parent table for associating with the specified parent.
// It returns a MachineNotFound error if the parent machine does not exist.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) CreateMachineWithParent(ctx context.Context, machineName, parentName machine.Name, nodeUUID string, machineUUID machine.UUID) error {
	return st.createMachine(ctx, createMachineArgs{
		name:        machineName,
		netNodeUUID: nodeUUID,
		machineUUID: machineUUID,
		parentName:  parentName,
	})
}

// createMachine creates or updates the specified machine.
// Adds a row to machine table, as well as a row to the net_node table.
// It returns the uuid of the created machine.
// It returns a MachineAlreadyExists error if a machine with the same name
// already exists.
func (st *State) createMachine(ctx context.Context, args createMachineArgs) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	mName := args.name

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}

	var machineExistsUUIDout machineExistsUUID
	machineUUIDStmt, err := st.Prepare(`
SELECT COUNT(m.uuid) AS &machineExistsUUID.count, &machineExistsUUID.uuid
FROM  machine AS m
WHERE name = $machineName.name;
`, machineNameParam, machineExistsUUIDout)
	if err != nil {
		return errors.Capture(err)
	}

	var nonce sql.Null[string]
	if args.nonce != nil {
		nonce = sql.Null[string]{
			V:     *args.nonce,
			Valid: true,
		}
	}

	lifeID, err := encodeLife(life.Alive)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for creating machine row.
	createParams := createMachine{
		UUID:        args.machineUUID.String(),
		Name:        mName.String(),
		NetNodeUUID: args.netNodeUUID,
		Nonce:       nonce,
		LifeID:      lifeID,
	}
	createMachineQuery := `
INSERT INTO machine (*)
VALUES ($createMachine.*)
`
	createMachineStmt, err := st.Prepare(createMachineQuery, createParams)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for creating net node row.
	createNodeQuery := `INSERT INTO net_node (uuid) VALUES ($createMachine.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNodeQuery, createParams)
	if err != nil {
		return errors.Capture(err)
	}

	now := st.clock.Now()

	instanceStatusInfo, err := status.EncodeCloudInstanceStatus(status.InstanceStatusPending)
	if err != nil {
		return errors.Capture(err)
	}
	machineStatusInfo, err := status.EncodeMachineStatus(status.MachineStatusPending)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid. If the machine already exists, return a
		// MachineAlreadyExists error.
		err := tx.Query(ctx, machineUUIDStmt, machineNameParam).Get(&machineExistsUUIDout)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying machine %q: %w", mName, err)
		} else if machineExistsUUIDout.Count != 0 {
			return errors.Errorf("machine %q", mName).Add(machineerrors.MachineAlreadyExists)
		}

		// Run query to create net node row.
		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Errorf("creating net node row for machine %q: %w", mName, err)
		}

		// Run query to create machine row.
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Errorf("creating machine row for machine %q: %w", mName, err)
		}
		// Ensure we always have an instance as well, otherwise we can't have
		// an associated status.
		if err := insertMachineInstance(ctx, tx, st, args.machineUUID); err != nil {
			return errors.Errorf("inserting machine instance for machine %q: %w", mName, err)
		}

		if err := st.insertMachineStatus(ctx, tx, mName, setStatusInfo{
			StatusID: machineStatusInfo,
			Updated:  ptr(now),
		}); err != nil {
			return errors.Errorf("inserting machine status for machine %q: %w", mName, err)
		}
		if err := insertMachineInstanceStatus(ctx, tx, st, args.machineUUID, setStatusInfo{
			StatusID: instanceStatusInfo,
			Updated:  ptr(now),
		}); err != nil {
			return errors.Errorf("inserting machine instance status for machine %q: %w", mName, err)
		}

		// If the parent name is empty, we don't need to associate a parent
		// machine.
		if args.parentName == "" {
			return nil
		}

		// Associate a parent machine if parentName is provided.
		if err := st.createParentMachineLink(ctx, tx, args); err != nil {
			return errors.Errorf("creating parent machine %q for machine %q: %w", args.parentName, mName, err)
		}

		return nil
	})
	if err != nil {
		return errors.Errorf("inserting machine %q: %w", mName, err)
	}
	return nil
}

func (st *State) createParentMachineLink(ctx context.Context, tx *sqlair.TX, args createMachineArgs) error {
	mName := args.name

	// Prepare query for associating/verifying parent machine.
	parentNameParam := machineName{Name: args.parentName}
	associateParentParam := machineParent{MachineUUID: args.machineUUID}
	associateParentQuery := `
INSERT INTO machine_parent (machine_uuid, parent_uuid)
VALUES ($machineParent.machine_uuid, $machineParent.parent_uuid)
`
	associateParentStmt, err := st.Prepare(associateParentQuery, associateParentParam)
	if err != nil {
		return errors.Capture(err)
	}

	var machineUUIDout machineUUID
	machineUUIDStmt, err := st.Prepare(`
SELECT &machineUUID.uuid
FROM   machine
WHERE  name = $machineName.name;
`, parentNameParam, machineUUIDout)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for verifying there's no grandparent.
	var outputMachineParent machineParent
	var inputParentMachineUUID machineUUID

	parentQuery := `
SELECT mp.parent_uuid AS &machineParent.parent_uuid
FROM   machine_parent AS mp
WHERE  machine_uuid = $machineUUID.uuid`
	parentQueryStmt, err := st.Prepare(parentQuery, outputMachineParent, inputParentMachineUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Query for the parent uuid.
	if err := tx.Query(ctx, machineUUIDStmt, parentNameParam).Get(&machineUUIDout); errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("parent machine %q for %q: %w", args.parentName, mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return errors.Errorf("querying parent machine %q for machine %q: %w", args.parentName, mName, err)
	}

	// Protect against grand-parenting.
	machineParentUUID := machineUUID{
		UUID: machineUUIDout.UUID,
	}

	var machineParent machineParent
	if err := tx.Query(ctx, parentQueryStmt, machineParentUUID).Get(&machineParent); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("querying for grandparent UUID for machine %q: %w", mName, err)
	} else if err == nil {
		// No error means we found a grandparent.
		return errors.Errorf("machine %q", mName).Add(machineerrors.GrandParentNotSupported)
	}

	// Run query to associate parent machine.
	associateParentParam.ParentUUID = machineUUIDout.UUID
	if err := tx.Query(ctx, associateParentStmt, associateParentParam).Run(); err != nil {
		return errors.Errorf("associating parent machine %q for machine %q: %w", args.parentName, mName, err)
	}

	return nil
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, mName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}
	machineUUIDParam := machineUUID{}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
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
		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUIDParam.UUID.String()); err != nil {
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

func (st *State) removeBasicMachineData(ctx context.Context, tx *sqlair.TX, machineUUID machineUUID) error {
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
	}

	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE machine_uuid = $machineUUID.uuid", table)
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
	db, err := st.DB()
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

// GetMachineStatus returns the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
// It returns a StatusNotSet if the status is not set.
// Idempotent.
func (st *State) GetMachineStatus(ctx context.Context, mName machine.Name) (status.StatusInfo[status.MachineStatusType], error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	nameIdent := machineName{Name: mName}

	var uuid machineUUID
	uuidQuery := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name;`
	uuidQueryStmt, err := st.Prepare(uuidQuery, nameIdent, uuid)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	var mStatus machineStatus
	statusQuery := `
SELECT &machineStatus.*
FROM v_machine_status AS st
WHERE st.machine_uuid = $machineUUID.uuid;
`
	statusCombinedQueryStmt, err := st.Prepare(statusQuery, uuid, mStatus)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, nameIdent).Get(&uuid)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("getting machine status for %q: %w", mName, machineerrors.MachineNotFound)
		} else if err != nil {
			return errors.Errorf("querying uuid for machine %q: %w", mName, err)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, uuid).Get(&mStatus)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine: %q: %w", mName, machineerrors.StatusNotSet)
		} else if err != nil {
			return errors.Errorf("querying machine status for machine %q: %w", mName, err)
		}

		return nil
	})

	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Capture(err)
	}

	// Convert the internal status id from the (machine_status_value table)
	// into the core status.Status type.
	machineStatus, err := status.DecodeMachineStatus(mStatus.Status)
	if err != nil {
		return status.StatusInfo[status.MachineStatusType]{}, errors.Errorf("decoding machine status for machine %q: %w", mName, err)
	}

	var since time.Time
	if mStatus.Updated.Valid {
		since = mStatus.Updated.Time
	} else {
		since = st.clock.Now()
	}

	var data []byte
	if len(mStatus.Data) > 0 {
		data = mStatus.Data
	}

	return status.StatusInfo[status.MachineStatusType]{
		Status:  machineStatus,
		Message: mStatus.Message,
		Since:   &since,
		Data:    data,
	}, nil
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
	machineUUID machine.UUID,
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
		MachineUUID    machine.UUID `db:"machine_uuid"`
		Version        string       `db:"version"`
		ArchitectureID int          `db:"architecture_id"`
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
	uuid machine.UUID,
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

// SetMachineStatus sets the status of the specified machine.
// It returns MachineNotFound if the machine does not exist.
func (st *State) SetMachineStatus(ctx context.Context, mName machine.Name, newStatus status.StatusInfo[status.MachineStatusType]) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	statusID, err := status.EncodeMachineStatus(newStatus.Status)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare the new status to be set.
	status := setStatusInfo{
		StatusID: statusID,
		Message:  newStatus.Message,
		Data:     newStatus.Data,
		Updated:  newStatus.Since,
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return st.insertMachineStatus(ctx, tx, mName, status)
	})
}

// SetMachineLife sets the life status of the specified machine.
// It returns a MachineNotFound if the provided machine doesn't exist.
func (st *State) SetMachineLife(ctx context.Context, mName machine.Name, life life.Life) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine UUID.
	machineNameParam := machineName{Name: mName}
	machineUUIDoutput := machineUUID{}
	uuidQuery := `SELECT &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, machineNameParam, machineUUIDoutput)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for updating machine life.
	machineLifeParam := machineLife{LifeID: life}
	query := `UPDATE machine SET life_id = $machineLife.life_id WHERE uuid = $machineLife.uuid`
	queryStmt, err := st.Prepare(query, machineLifeParam)
	if err != nil {
		return errors.Capture(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for machine uuid, return MachineNotFound if machine doesn't exist.
		err := tx.Query(ctx, uuidQueryStmt, machineNameParam).Get(&machineUUIDoutput)
		if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		if err != nil {
			return errors.Errorf("querying UUID for machine %q: %w", mName, err)
		}

		// Update machine life status.
		machineLifeParam.UUID = machineUUIDoutput.UUID
		err = tx.Query(ctx, queryStmt, machineLifeParam).Run()
		if err != nil {
			return errors.Errorf("setting life for machine %q: %w", mName, err)
		}
		return nil
	})
}

// IsMachineController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) IsMachineController(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT &count.*
FROM   v_machine_is_controller
WHERE  machine_uuid = $machineUUID.uuid
`
	queryStmt, err := st.Prepare(query, machineUUID{}, result)
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
	db, err := st.DB()
	if err != nil {
		return false, errors.Capture(err)
	}

	var result count
	query := `
SELECT     COUNT(m.uuid) AS &count.count
FROM       machine AS m
JOIN  machine_manual AS mm ON m.uuid = mm.machine_uuid
WHERE      m.uuid = $machineUUID.uuid
`
	queryStmt, err := st.Prepare(query, machineUUID{}, count{})
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

func (st *State) getMachineUUIDFromName(ctx context.Context, tx *sqlair.TX, mName machine.Name) (machineUUID, error) {
	machineNameParam := machineName{Name: mName}
	machineUUIDoutput := machineUUID{}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, machineUUIDoutput)
	if err != nil {
		return machineUUID{}, errors.Capture(err)
	}

	if err := tx.Query(ctx, queryStmt, machineNameParam).Get(&machineUUIDoutput); errors.Is(err, sqlair.ErrNoRows) {
		return machineUUID{}, errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return machineUUID{}, errors.Errorf("querying UUID for machine %q: %w", mName, err)
	}
	return machineUUIDoutput, nil
}

// AllMachineNames retrieves the names of all machines in the model.
func (st *State) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	db, err := st.DB()
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
func (st *State) GetMachineParentUUID(ctx context.Context, uuid machine.UUID) (machine.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	// Prepare query for checking that the machine exists.
	currentMachineUUID := machineUUID{UUID: uuid}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE uuid = $machineUUID.uuid`
	queryStmt, err := st.Prepare(query, currentMachineUUID)
	if err != nil {
		return "", errors.Capture(err)
	}

	// Prepare query for parent UUID.
	var parentUUID machine.UUID
	parentUUIDParam := machineParent{}
	parentQuery := `
SELECT parent_uuid AS &machineParent.parent_uuid
FROM machine_parent WHERE machine_uuid = $machineUUID.uuid`
	parentQueryStmt, err := st.Prepare(parentQuery, currentMachineUUID, parentUUIDParam)
	if err != nil {
		return "", errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		outUUID := machineUUID{} // This value doesn't really matter, it is just a way to check existence
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

		parentUUID = parentUUIDParam.ParentUUID

		return nil
	})
	if err != nil {
		return parentUUID, errors.Errorf("getting parent UUID for machine %q: %w", uuid, err)
	}
	return parentUUID, nil
}

// MarkMachineForRemoval marks the specified machine for removal.
// It returns NotFound if the machine does not exist.
// TODO(cderici): use machineerrors.MachineNotFound on rebase after #17759
// lands.
// Idempotent.
func (st *State) MarkMachineForRemoval(ctx context.Context, mName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for getting the machine UUID.
	machineNameParam := machineName{Name: mName}
	markForRemovalUUID := machineMarkForRemoval{}
	machineUUIDQuery := `SELECT uuid AS &machineMarkForRemoval.machine_uuid FROM machine WHERE name = $machineName.name`
	machineUUIDStmt, err := st.Prepare(machineUUIDQuery, machineNameParam, markForRemovalUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for adding the machine to the machine_removals table.
	markForRemovalUpdateQuery := `
INSERT OR IGNORE INTO machine_removals (machine_uuid)
VALUES ($machineMarkForRemoval.machine_uuid)`
	markForRemovalStmt, err := st.Prepare(markForRemovalUpdateQuery, markForRemovalUUID)
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine UUID.
		err := tx.Query(ctx, machineUUIDStmt, machineNameParam).Get(&markForRemovalUUID)
		if errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
		}
		if err != nil {
			return errors.Errorf("querying UUID for machine %q: %w", mName, err)
		}

		// Run query for adding the machine to the removals table.
		return tx.Query(ctx, markForRemovalStmt, markForRemovalUUID).Run()
	})
	if err != nil {
		return errors.Errorf("marking machine %q for removal: %w", mName, err)
	}
	return nil
}

// GetAllMachineRemovals returns the UUIDs of all of the machines that need to
// be removed but need provider-level cleanup.
func (st *State) GetAllMachineRemovals(ctx context.Context) ([]machine.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	// Prepare query for uuids of all machines marked for removal.
	markForRemovalParam := machineMarkForRemoval{}
	machinesMarkedForRemovalQuery := `SELECT &machineMarkForRemoval.machine_uuid FROM machine_removals`
	machinesMarkedForRemovalStmt, err := st.Prepare(machinesMarkedForRemovalQuery, markForRemovalParam)
	if err != nil {
		return nil, errors.Capture(err)
	}

	var results []machineMarkForRemoval
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Run query to get all machines marked for removal.
		err := tx.Query(ctx, machinesMarkedForRemovalStmt).GetAll(&results)
		// No errors if there's no machine marked for removal.
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		return err
	})

	if err != nil {
		return nil, errors.Errorf("querying all machines marked for removal: %w", err)
	}

	// Transform the results ([]machineUUID) into a slice of machine UUIDs.
	machineUUIDs := transform.Slice(
		results,
		machineMarkForRemoval.uuidSliceTransform,
	)

	return machineUUIDs, nil
}

// GetMachineUUID returns the UUID of a machine identified by its name.
// It returns a MachineNotFound if the machine does not exist.
func (st *State) GetMachineUUID(ctx context.Context, name machine.Name) (machine.UUID, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Capture(err)
	}

	var uuid machineUUID
	currentMachineName := machineName{Name: name}
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

// ShouldKeepInstance reports whether a machine, when removed from Juju, should cause
// the corresponding cloud instance to be stopped.
func (st *State) ShouldKeepInstance(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB()
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
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for machine uuid.
	machineUUID := machineUUID{}
	machineNameParam := machineName{Name: mName}
	machineExistsQuery := `
SELECT uuid AS &machineUUID.uuid
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
func (st *State) AppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID) ([]string, error) {
	db, err := st.DB()
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
func (st *State) SetAppliedLXDProfileNames(ctx context.Context, mUUID machine.UUID, profileNames []string) error {
	if len(profileNames) == 0 {
		return nil
	}

	db, err := st.DB()
	if err != nil {
		return errors.Errorf("cannot get database to set lxd profiles %v for machine %q: %w", profileNames, mUUID, err)
	}

	queryMachineUUID := machineUUID{UUID: mUUID}
	checkMachineExistsStmt, err := st.Prepare(`
SELECT uuid AS &machineUUID.uuid
FROM   machine
WHERE  machine.uuid = $machineUUID.uuid`, queryMachineUUID)
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
		`DELETE FROM machine_lxd_profile WHERE machine_uuid = $machineUUID.uuid`,
		machineUUID{},
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
		var machineExists machineUUID
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
func (st *State) GetNamesForUUIDs(ctx context.Context, machineUUIDs []string) (map[string]machine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Errorf("cannot get database find names for machines %q: %w", machineUUIDs, err)
	}

	type nameAndUUID availabilityZoneName
	type uuids []string

	stmt, err := st.Prepare(`
SELECT &nameAndUUID.*
FROM   machine
WHERE  machine.uuid IN ($uuids[:])`, nameAndUUID{}, uuids{})
	if err != nil {
		return nil, errors.Capture(err)
	}

	var namesAndUUIDs []nameAndUUID
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, stmt, uuids(machineUUIDs)).GetAll(&namesAndUUIDs)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		} else if errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		}
		return nil
	})

	return transform.SliceToMap(namesAndUUIDs, func(n nameAndUUID) (string, machine.Name) {
		return n.UUID, machine.Name(n.Name)
	}), err
}

// GetMachineArchesForApplication returns a map of machine names to their
// instance IDs. This will ignore non-provisioned machines or container
// machines.
func (st *State) GetAllProvisionedMachineInstanceID(ctx context.Context) (map[string]string, error) {
	db, err := st.DB()
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

	instanceIDs := make(map[string]string)
	for _, result := range results {
		if result.IsContainer > 0 || result.MachineName == "" || result.InstanceID == "" {
			continue
		}
		instanceIDs[result.MachineName] = result.InstanceID
	}
	return instanceIDs, nil
}

// SetMachineHostname sets the hostname for the given machine.
// Also updates the agent_started_at timestamp.
func (st *State) SetMachineHostname(ctx context.Context, mUUID machine.UUID, hostname string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Capture(err)
	}

	currentMachineUUID := machineUUID{UUID: mUUID}
	query := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE uuid = $machineUUID.uuid`
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
SET
    hostname = $machineHostName.hostname,
	agent_started_at = $machineHostName.agent_started_at
WHERE 
	uuid = $machineUUID.uuid`
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
func (st *State) GetSupportedContainersTypes(ctx context.Context, mUUID machine.UUID) ([]string, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Capture(err)
	}

	currentMachineUUID := machineUUID{UUID: mUUID}
	query := `
SELECT ct.value AS &containerType.container_type
FROM machine AS m
LEFT JOIN container_type AS ct ON m.container_type_id = ct.id
WHERE uuid = $machineUUID.uuid
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

// InitialWatchModelMachinesStatement returns the table and the initial watch
// statement for watching life changes of non-container machines.
func (*State) InitialWatchModelMachinesStatement() (string, string) {
	return "machine", "SELECT name FROM machine WHERE name NOT LIKE '%/%'"
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (*State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT name FROM machine"
}

func (st *State) insertMachineStatus(ctx context.Context, tx *sqlair.TX, mName machine.Name, status setStatusInfo) error {
	nameIdent := machineName{Name: mName}

	var mUUID machineUUID
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name;`
	queryMachineStmt, err := st.Prepare(queryMachine, nameIdent, mUUID)
	if err != nil {
		return errors.Capture(err)
	}

	// Prepare query for setting machine status
	statusQuery := `
INSERT INTO machine_status (*)
VALUES ($setMachineStatus.*)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET
  	status_id = excluded.status_id,
	message = excluded.message,
	updated_at = excluded.updated_at,
	data = excluded.data;
`
	statusQueryStmt, err := st.Prepare(statusQuery, setMachineStatus{})
	if err != nil {
		return errors.Capture(err)
	}

	// Query for the machine uuid.
	if err := tx.Query(ctx, queryMachineStmt, nameIdent).Get(&mUUID); errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("machine %q: %w", mName, machineerrors.MachineNotFound)
	} else if err != nil {
		return errors.Errorf("querying uuid for machine %q: %w", mName, err)
	}

	// Query for setting the machine status.
	err = tx.Query(ctx, statusQueryStmt, setMachineStatus{
		MachineUUID: mUUID.UUID,
		StatusID:    status.StatusID,
		Message:     status.Message,
		Data:        status.Data,
		Updated:     status.Updated,
	}).Run()
	if err != nil {
		return errors.Errorf("setting machine status for machine %q: %w", mName, err)
	}

	return nil
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

func ptr[T any](v T) *T {
	return &v
}
