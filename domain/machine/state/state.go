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
		// For now, we just care if the minimal machine row already exists.
		if err == nil {
			return nil
		}
		if err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "querying machine %q", machineName)
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

	// Prepare query for machine uuid.
	machineNameParam := machineName{Name: mName}
	machineUUIDParam := machineUUID{}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting machine row.
	deleteMachine := `DELETE FROM machine WHERE name = $machineName.name`
	deleteMachineStmt, err := st.Prepare(deleteMachine, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting net node row.
	deleteNode := `
DELETE FROM net_node WHERE uuid IN
(SELECT net_node_uuid FROM machine WHERE name = $machineName.name)
`
	deleteNodeStmt, err := st.Prepare(deleteNode, machineNameParam)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for deleting status and status data for the machine.
	deleteStatus := `DELETE FROM machine_status WHERE machine_uuid = $machineUUID.uuid`
	deleteStatusStmt, err := st.Prepare(deleteStatus, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	deleteStatusData := `DELETE FROM machine_status_data WHERE machine_uuid = $machineUUID.uuid`
	deleteStatusDataStmt, err := st.Prepare(deleteStatusData, machineUUIDParam)
	if err != nil {
		return errors.Trace(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		err = tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&machineUUIDParam)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return machineerrors.NotFound
			}
			return errors.Annotatef(err, "looking up UUID for machine %q", mName)
		}

		// Remove block devices for the machine.
		if err := blockdevice.RemoveMachineBlockDevices(ctx, tx, machineUUIDParam.UUID); err != nil {
			return errors.Annotatef(err, "deleting block devices for machine %q", mName)
		}

		// Remove the status for the machine. No need to return error if no
		// status is set for the machine while deleting.
		if err := tx.Query(ctx, deleteStatusStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "deleting status for machine %q", mName)
		}

		// Remove the status data for the machine. No need to return error if no
		// status data is set for the machine.
		if err := tx.Query(ctx, deleteStatusDataStmt, machineUUIDParam).Run(); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Annotatef(err, "deleting status data for machine %q", mName)
		}

		// Remove the machine.
		if err := tx.Query(ctx, deleteMachineStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting machine %q", mName)
		}

		// Remove the net node for the machine.
		if err := tx.Query(ctx, deleteNodeStmt, machineNameParam).Run(); err != nil {
			return errors.Annotatef(err, "deleting net node for machine  %q", mName)
		}

		return nil
	})
	return errors.Annotatef(err, "deleting machine %q", mName)
}

// InitialWatchStatement returns the table and the initial watch statement
// for the machines.
func (st *State) InitialWatchStatement() (string, string) {
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
			if errors.Is(err, sqlair.ErrNoRows) {
				return machineerrors.NotFound
			}
			return errors.Annotatef(err, "looking up life for machine %q", mName)
		}

		lifeResult = result.LifeID

		return nil
	})
	if err != nil {
		return nil, errors.Annotatef(err, "getting life status for machine %q", mName)
	}
	return &lifeResult, nil
}

// GetMachineStatus returns the status of the specified machine.
// It returns NotFound if the machine does not exist.
// It returns a StatusNotSet if the status is not set.
// Idempotent.
func (st *State) GetMachineStatus(ctx context.Context, mName machine.Name) (status.StatusInfo, error) {
	db, err := st.DB()
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Prepare query for machine uuid (to be used in machine status and status
	// data tables)
	machineNameParam := machineName{Name: mName}
	machineUUIDout := machineUUID{}
	uuidQuery := `SELECT uuid AS &machineUUID.uuid FROM machine WHERE name = $machineName.name`
	uuidQueryStmt, err := st.Prepare(uuidQuery, machineNameParam, machineUUIDout)
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Prepare query for combined machine status and the status data (to get
	// them both in one transaction, as this a a relatively frequent retrieval).
	machineStatusDataParam := machineStatusWithData{}
	statusCombinedQuery := `
SELECT (st.status,
		st.message,
		st.updated_at,
		st_data.key,
		st_data.data) as (&machineStatusWithData.*)
FROM 	machine_status AS st
		LEFT JOIN machine_status_data AS st_data
		ON st.machine_uuid = st_data.machine_uuid
WHERE st.machine_uuid = $machineUUID.uuid`
	statusCombinedQueryStmt, err := st.Prepare(statusCombinedQuery, machineUUIDout, machineStatusDataParam)
	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	var machineStatusWithAllData machineStatusData
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid
		err := tx.Query(ctx, uuidQueryStmt, machineNameParam).Get(&machineUUIDout)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.NotFoundf("machine %q", mName)
			}
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for the machine cloud instance status and status data combined
		err = tx.Query(ctx, statusCombinedQueryStmt, machineUUIDout).GetAll(&machineStatusWithAllData)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.Annotatef(machineerrors.StatusNotSet, "machine: %q", mName)
			}
			return errors.Annotatef(err, "querying machine status for machine %q", mName)
		}

		return nil
	})

	if err != nil {
		return status.StatusInfo{}, errors.Trace(err)
	}

	// Transform the status data slice into a status.Data map.
	statusDataResult := transform.SliceToMap(machineStatusWithAllData, func(d machineStatusWithData) (string, interface{}) {
		return d.Key, d.Data
	})

	machineStatus := status.StatusInfo{
		Message: machineStatusWithAllData[0].Message,
		Since:   machineStatusWithAllData[0].Updated,
		Data:    statusDataResult,
	}

	// Convert the internal status id from the (machine_status_values table)
	// into the core status.Status type.
	machineStatus.Status = machineStatusWithAllData[0].toCoreMachineStatusValue()

	return machineStatus, nil
}

// SetMachineStatus sets the status of the specified machine.
// It returns NotFound if the machine does not exist.
func (st *State) SetMachineStatus(ctx context.Context, mName machine.Name, newStatus status.StatusInfo) error {
	db, err := st.DB()
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare the new status to be set.
	machineStatus := machineStatusWithData{}

	machineStatus.Status = fromCoreMachineStatusValue(newStatus.Status)
	machineStatus.Message = newStatus.Message
	machineStatus.Updated = newStatus.Since
	machineStatusData := transform.MapToSlice(newStatus.Data, func(key string, value interface{}) []machineStatusWithData {
		return []machineStatusWithData{{Key: key, Data: value.(string)}}
	})

	// Prepare query for machine uuid
	machineNameParam := machineName{Name: mName}
	mUUID := machineUUID{}
	queryMachine := `SELECT uuid AS &machineUUID.* FROM machine WHERE name = $machineName.name`
	queryMachineStmt, err := st.Prepare(queryMachine, machineNameParam, mUUID)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting machine status
	statusQuery := `
INSERT INTO machine_status (machine_uuid, status, message, updated_at)
VALUES ($machineUUID.uuid, $machineStatusWithData.status, $machineStatusWithData.message, $machineStatusWithData.updated_at)
  ON CONFLICT (machine_uuid)
  DO UPDATE SET status = excluded.status, message = excluded.message, updated_at = excluded.updated_at
`
	statusQueryStmt, err := st.Prepare(statusQuery, mUUID, machineStatus)
	if err != nil {
		return errors.Trace(err)
	}

	// Prepare query for setting machine status data
	statusDataQuery := `
INSERT INTO machine_status_data (machine_uuid, "key", data)
VALUES ($machineUUID.uuid, $machineStatusWithData.key, $machineStatusWithData.data)
  ON CONFLICT (machine_uuid, "key") DO UPDATE SET data = excluded.data
`
	statusDataQueryStmt, err := st.Prepare(statusDataQuery, mUUID, machineStatus)
	if err != nil {
		return errors.Trace(err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// Query for the machine uuid.
		err := tx.Query(ctx, queryMachineStmt, machineNameParam).Get(&mUUID)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return errors.NotFoundf("machine %q", mName)
			}
			return errors.Annotatef(err, "querying uuid for machine %q", mName)
		}

		// Query for setting the machine status.
		err = tx.Query(ctx, statusQueryStmt, mUUID, machineStatus).Run()
		if err != nil {
			return errors.Annotatef(err, "setting machine status for machine %q", mName)
		}

		// Query for setting the machine status data if machineStatusData is not
		// empty.
		if len(machineStatusData) > 0 {
			err = tx.Query(ctx, statusDataQueryStmt, mUUID, machineStatusData).Run()
			if err != nil {
				return errors.Annotatef(err, "setting machine status data for machine %q", mName)
			}
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
				return machineerrors.NotFound
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
			if errors.Is(err, sqlair.ErrNoRows) {
				return machineerrors.NotFound
			}
			return errors.Annotatef(err, "querying if machine %q is a controller", mName)
		}
		return nil
	})
	if err != nil {
		return false, errors.Annotatef(err, "checking if machine %q is a controller", mName)
	}

	return result.IsController, nil
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
		err := tx.Query(ctx, queryStmt).GetAll(&results)
		if err != nil {
			if errors.Is(err, sqlair.ErrNoRows) {
				return nil
			}
			return errors.Annotate(err, "querying all machines")
		}
		return nil
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Transform the results ([]machineName) into a slice of machine.Name.
	machineNames := transform.Slice[machineName, machine.Name](
		results,
		func(r machineName) machine.Name { return r.Name },
	)

	return machineNames, nil
}
