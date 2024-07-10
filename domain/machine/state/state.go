// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/collections/transform"
	"github.com/juju/errors"

	coredb "github.com/juju/juju/core/database"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/domain"
	blockdevice "github.com/juju/juju/domain/blockdevice/state"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/internal/database"
)

// State describes retrieval and persistence methods for storage.
type State struct {
	*domain.StateBase
	logger logger.Logger
}

// NewState returns a new state reference.
func NewState(factory coredb.TxnRunnerFactory, logger logger.Logger) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
		logger:    logger,
	}
}

// CreateMachine creates or updates the specified machine.
// TODO - this just creates a minimal row for now.
func (st *State) CreateMachine(ctx context.Context, machineName machine.Name, nodeUUID, machineUUID string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineNameParam := sqlair.M{"name": machineName}
	query := `SELECT &M.uuid FROM machine WHERE name = $M.name`
	queryStmt, err := st.Prepare(query, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createMachine := `
INSERT INTO machine (uuid, net_node_uuid, name, life_id)
VALUES ($M.machine_uuid, $M.net_node_uuid, $M.name, $M.life_id)
`
	createMachineStmt, err := st.Prepare(createMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createNode := `INSERT INTO net_node (uuid) VALUES ($M.net_node_uuid)`
	createNodeStmt, err := st.Prepare(createNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	createParams := sqlair.M{
		"machine_uuid":  machineUUID,
		"net_node_uuid": nodeUUID,
		"name":          machineName,
		"life_id":       life.Alive,
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := sqlair.M{}
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if err != nil {
			if !errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(err, "querying machine %q", machineName)
			}
		}
		// For now, we just care if the minimal machine row already exists.
		if err == nil {
			return nil
		}

		if err := tx.Query(ctx, createNodeStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating net node row for machine %q", machineName)
		}
		if err := tx.Query(ctx, createMachineStmt, createParams).Run(); err != nil {
			return errors.Annotatef(err, "creating machine row for machine %q", machineName)
		}
		return nil
	})
	return errors.Annotatef(err, "inserting machine %q", machineName)
}

// DeleteMachine deletes the specified machine and any dependent child records.
// TODO - this just deals with child block devices for now.
func (st *State) DeleteMachine(ctx context.Context, mName machine.Name) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, machineUUID{})
	if err != nil {
		return errors.Trace(err)
	}

	deleteMachine := `DELETE FROM machine WHERE name = $machineName.name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE name = $machineName.name)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineUUID{}
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&result)
		// Machine already deleted is a no op.
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "looking up UUID for machine %q", mName)
		}

		machineUUID := result.UUID

		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", mName)
		}

		if err := tx.Query(ctx, deleteMachineStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", mName)
		}
		if err := tx.Query(ctx, deleteNodeStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", mName)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", mName)
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (s *State) InitialWatchStatement() (string, string) {
	return "machine", "SELECT name FROM machine"
}

// GetMachineLife returns the life status of the specified machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) GetMachineLife(ctx context.Context, mName machine.Name) (*life.Life, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	queryForLife := `SELECT life_id as &machineLife.life_id FROM machine WHERE name = $machineName.name`
	lifeStmt, err := st.Prepare(queryForLife, machineNameParam, machineLife{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var lifeResult life.Life
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		result := machineLife{}
		err := tx.Query(ctx, lifeStmt, machineNameParam).Get(&result)
		if err != nil {
			return errors.Annotatef(err, "looking up life for machine %q", mName)
		}

		lifeResult = result.LifeID

		return nil
	})

	if err != nil && errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.NotFoundf("machine %q", mName)
	}

	return &lifeResult, errors.Annotatef(err, "getting life status for machines %q", mName)
}

// GetMachineStatus returns the status of the specified machine.
// It returns a StatusNotSet if the status is not set.
// Idempotent.
func (st *State) GetMachineStatus(ctx context.Context, mName machine.Name) (status.Status, error) {
	db, err := st.DB()
	if err != nil {
		return "", errors.Trace(err)
	}
	machineStatusParam := machineInstanceStatus{Name: mName}
	statusQuery := `
SELECT ms.status as &machineInstanceStatus.status
FROM machine as m
	JOIN machine_status as ms ON m.uuid = ms.machine_uuid
WHERE m.name = $machineInstanceStatus.name;
`

	statusQueryStmt, err := st.Prepare(statusQuery, machineStatusParam)
	if err != nil {
		return "", errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, statusQueryStmt, machineStatusParam).Get(&machineStatusParam)
		if err != nil {
			return errors.Annotatef(err, "querying machine status for machine %q", mName)
		}
		return nil
	})

	if err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return "", errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
		}
		return "", errors.Trace(err)
	}
	internalStatus := machineStatusParam.Status
	// Convert the internal status id from the (machine_status_values table)
	// into the core status.Status type.
	var machineStatus status.Status
	switch internalStatus {
	case 0:
		machineStatus = status.Error
	case 1:
		machineStatus = status.Started
	case 2:
		machineStatus = status.Pending
	case 3:
		machineStatus = status.Stopped
	case 4:
		machineStatus = status.Down
	}
	return machineStatus, nil
}

// SetMachineStatus sets the status of the specified machine.
func (st *State) SetMachineStatus(ctx context.Context, mName machine.Name, newStatus status.Status) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	var iStatus int
	switch newStatus {
	case status.Error:
		iStatus = 0
	case status.Started:
		iStatus = 1
	case status.Pending:
		iStatus = 2
	case status.Stopped:
		iStatus = 3
	case status.Down:
		iStatus = 4
	}
	machineStatus := machineInstanceStatus{
		Name:   mName,
		Status: iStatus,
	}

	mUUID := instanceTag{}
	queryMachine := `SELECT uuid AS &instanceTag.machine_uuid FROM machine WHERE name = $machineInstanceStatus.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineStatus, mUUID)
	if err != nil {
		return errors.Trace(err)
	}

	statusQuery := `
INSERT INTO machine_status (*)
VALUES ($instanceTag.machine_uuid, $machineInstanceStatus.status)
`
	statusQueryStmt, err := st.Prepare(statusQuery, mUUID, machineStatus)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryMachineStmt, machineStatus).Get(&mUUID)
		if err != nil {
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}
		err = tx.Query(ctx, statusQueryStmt, mUUID, machineStatus).Run()
		if err != nil {
			return errors.Annotatef(err, "setting machine status for machine %q", mName)
		}
		return nil
	})
}

// SetMachineLife sets the life status of the specified machine.
// It returns a NotFound if the provided machine doesn't exist.
func (st *State) SetMachineLife(ctx context.Context, mName machine.Name, life life.Life) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for machine UUID.
	machineNameParam := machineName{Name: mName}
	machineUUIDoutput := machineUUID{}
	uuidQuery := `SELECT &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, machineNameParam, machineUUIDoutput)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for updating machine life.
	machineLifeParam := machineLife{LifeID: life}
	query := `UPDATE machine SET life_id = $machineLife.life_id WHERE uuid = $machineLife.uuid`
	queryStmt, err := st.Prepare(query, machineLifeParam)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for machine uuid, return NotFound if machine doesn't exist.
		err := tx.Query(ctx, uuidQueryStmt, machineNameParam).Get(&machineUUIDoutput)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.NotFoundf("machine not found %q", mName)
			}
			return errors.Annotatef(err, "querying UUID for machine %q", mName)
		}

		// Update machine life status.
		machineLifeParam.UUID = machineUUIDoutput.UUID
		err = tx.Query(ctx, queryStmt, machineLifeParam).Run()
		if err != nil {
			return errors.Annotatef(err, "setting life for machine %q", mName)
		}
		return nil
	})
}

// IsController returns whether the machine is a controller machine.
// It returns a NotFound if the given machine doesn't exist.
func (st *State) IsController(ctx context.Context, mName machine.Name) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	machineNameParam := machineName{Name: mName}
	result := machineIsController{}
	query := `SELECT &machineIsController.is_controller FROM machine WHERE name = $machineName.name`
	queryStmt, err := st.Prepare(query, machineNameParam, result)
	if err != nil {
		return false, errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err := tx.Query(ctx, queryStmt, machineNameParam).Get(&result)
		if err != nil {
			return errors.Annotatef(err, "querying if machine %q is a controller", mName)
		}
		return nil
	})
	if err != nil && errors.Is(err, sqlair.ErrNoRows) {
		return false, errors.NotFoundf("machine %q", mName)
	}

	return result.IsController, errors.Annotatef(err, "checking if machine %q is a controller", mName)
}

// AllMachineNames retrieves the names of all machines in the model.
func (st *State) AllMachineNames(ctx context.Context) ([]machine.Name, error) {
	db, err := st.DB()
	if err != nil {
		return nil, errors.Trace(err)
	}

	query := `SELECT name AS &machineName.* FROM machine`
	queryStmt, err := st.Prepare(query, machineName{})
	if err != nil {
		return nil, errors.Trace(err)
	}

	var results []machineName
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, queryStmt).GetAll(&results)
	})
	if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Annotate(err, "querying all machines")
	}

	// Transform the results ([]machineName) into a slice of machine.Name.
	machineNames := transform.Slice[machineName, machine.Name](results, func(r machineName) machine.Name { return r.Name })

	return machineNames, nil
}

type machineReboot struct {
	UUID string `db:"uuid"`
}

// RequireMachineReboot sets the machine referenced by its UUID as requiring a reboot.
//
// Reboot requests are handled through the "machine_requires_reboot" table which contains only
// machine UUID for which a reboot has been requested.
// This function is idempotent.
func (st *State) RequireMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	setRebootFlag := `INSERT INTO machine_requires_reboot (machine_uuid) VALUES ($machineReboot.uuid)`
	setRebootFlagStmt, err := sqlair.Prepare(setRebootFlag, machineReboot{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, setRebootFlagStmt, machineReboot{uuid}).Run()
	})
	if database.IsErrConstraintPrimaryKey(err) {
		// if the same uuid is added twice, do nothing (idempotency)
		return nil
	}
	return errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}

// CancelMachineReboot cancels the reboot of the machine referenced by its UUID if it has
// previously been required.
//
// It basically removes the uuid from the "machine_requires_reboot" table if present.
// This function is idempotent.
func (st *State) CancelMachineReboot(ctx context.Context, uuid string) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}
	unsetRebootFlag := `DELETE FROM machine_requires_reboot WHERE machine_uuid = $machineReboot.uuid`
	unsetRebootFlagStmt, err := sqlair.Prepare(unsetRebootFlag, machineReboot{})
	if err != nil {
		return errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, unsetRebootFlagStmt, machineReboot{uuid}).Run()
	})
	return errors.Annotatef(err, "cancelling reboot of machine %q", uuid)
}

// IsMachineRebootRequired checks if the specified machine requires a reboot.
//
// It queries the "machine_requires_reboot" table for the machine UUID to determine if a reboot is required.
// Returns a boolean value indicating if a reboot is required, and an error if any occur during the process.
func (st *State) IsMachineRebootRequired(ctx context.Context, uuid string) (bool, error) {
	db, err := st.DB()
	if err != nil {
		return false, errors.Trace(err)
	}

	var isRebootRequired bool
	isRebootFlag := `SELECT machine_uuid as &machineReboot.uuid  FROM machine_requires_reboot WHERE machine_uuid = $machineReboot.uuid`
	isRebootFlagStmt, err := sqlair.Prepare(isRebootFlag, machineReboot{})
	if err != nil {
		return false, errors.Trace(err)
	}
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var results machineReboot
		err := tx.Query(ctx, isRebootFlagStmt, machineReboot{uuid}).Get(&results)
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Trace(err)
		}
		isRebootRequired = !errors.Is(err, sqlair.ErrNoRows)
		return nil
	})

	return isRebootRequired, errors.Annotatef(err, "requiring reboot of machine %q", uuid)
}
