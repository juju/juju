// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/machine"
	corestatus "github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddExecOperation creates an exec operation with tasks for various machines
// and units, using the provided parameters.
func (s *State) AddExecOperation(
	ctx context.Context,
	operationUUID internaluuid.UUID,
	target operation.ReceiversWithoutLeader,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = s.createExecOperation(ctx, tx, operationUUID.String(), target, args)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation: %w", err)
	}

	return result, nil
}

// AddExecOperationOnAllMachines creates an exec operation with tasks based
// on the provided parameters on all machines.
func (s *State) AddExecOperationOnAllMachines(
	ctx context.Context,
	operationUUID internaluuid.UUID,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machines, err := s.getAllMachines(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}

		target := operation.ReceiversWithoutLeader{
			Machines: machines,
		}

		result, err = s.createExecOperation(ctx, tx, operationUUID.String(), target, args)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation on all machines: %w", err)
	}

	return result, nil
}

// AddActionOperation creates an action operation with tasks for various
// units using the provided parameters.
func (s *State) AddActionOperation(ctx context.Context,
	operationUUID internaluuid.UUID,
	targetUnits []coreunit.Name,
	args operation.TaskArgs,
) (operation.RunResult, error) {
	// We need at least one unit target task.
	if len(targetUnits) == 0 {
		return operation.RunResult{}, errors.Errorf("no target units provided")
	}

	db, err := s.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		// Generate the operation ID.
		operationID, err := sequencestate.NextValue(ctx, s, tx, operation.OperationSequenceNamespace)
		if err != nil {
			return errors.Errorf("generating operation ID: %w", err)
		}
		result.OperationID = strconv.Itoa(int(operationID))

		// Insert the operation first.
		err = s.insertOperation(ctx, tx, insertOperation{
			UUID:           operationUUID.String(),
			OperationID:    fmt.Sprintf("%d", operationID),
			Summary:        fmt.Sprintf("action %s", args.ActionName),
			EnqueuedAt:     time.Now().UTC(),
			Parallel:       args.IsParallel,
			ExecutionGroup: args.ExecutionGroup,
		})
		if err != nil {
			return errors.Errorf("inserting operation: %w", err)
		}

		// Insert operation action.
		// We need to find the first valid unit to get the charm UUID, needed
		// for the operation action.
		var charmUUID string
		var validUnitFound bool
		for _, targetUnit := range targetUnits {
			charmUUID, err = s.getCharmUUIDByUnit(ctx, tx, targetUnit)
			if err == nil {
				validUnitFound = true
				break
			}
		}
		if !validUnitFound {
			return errors.Errorf("no valid unit found for the action %s", args.ActionName)
		}

		err = s.insertOperationAction(ctx, tx, operationUUID.String(), charmUUID, args.ActionName)
		if err != nil {
			return errors.Errorf("inserting operation action: %w", err)
		}

		// Insert operation parameters.
		for key, value := range args.Parameters {
			err = s.insertOperationParameter(ctx, tx, operationUUID.String(), key, fmt.Sprintf("%v", value))
			if err != nil {
				return errors.Errorf("inserting parameter %s: %w", key, err)
			}
		}

		// Finally insert all the unit tasks, one per target.
		// NOTE: We don't return the error here because we insert all the tasks
		// without rollbacking the transaction. The errors are returned as part
		// of the (RunResult) response on the individual tasks.
		result.Units = make([]operation.UnitTaskResult, len(targetUnits))
		for i, targetUnit := range targetUnits {
			// Create the task UUID.
			taskUUID, err := internaluuid.NewUUID()
			if err != nil {
				result.Units[i] = operation.UnitTaskResult{
					ReceiverName: targetUnit,
					TaskInfo: operation.TaskInfo{
						Error: errors.Errorf("generating task UUID: %w", err),
					},
				}
				continue
			}
			result.Units[i] = s.createUnitTask(ctx, tx, operationUUID.String(), taskUUID.String(), targetUnit)
		}
		return nil
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting action operation: %w", err)
	}

	return result, nil
}

// createExecOperation creates the exec operation for the provided receivers.
func (s *State) createExecOperation(
	ctx context.Context,
	tx *sqlair.TX,
	operationUUID string,
	target operation.ReceiversWithoutLeader,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	// Generate the operation ID.
	operationID, err := sequencestate.NextValue(ctx, s, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation ID: %w", err)
	}

	now := time.Now().UTC()
	// Insert the operation first.
	err = s.insertOperation(ctx, tx, insertOperation{
		UUID:           operationUUID,
		OperationID:    strconv.Itoa(int(operationID)),
		Summary:        fmt.Sprintf("exec %s", args.Command),
		EnqueuedAt:     now,
		Parallel:       args.Parallel,
		ExecutionGroup: args.ExecutionGroup,
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("inserting operation: %w", err)
	}

	// Exec operations have command and timeout parameters.
	err = s.insertOperationParameter(ctx, tx, operationUUID, "command", args.Command)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("inserting command parameter: %w", err)
	}
	err = s.insertOperationParameter(ctx, tx, operationUUID, "timeout", args.Timeout.String())
	if err != nil {
		return operation.RunResult{}, errors.Errorf("inserting timeout parameter: %w", err)
	}

	var result operation.RunResult
	result.OperationID = fmt.Sprintf("%d", operationID)

	// Insert tasks.
	// NOTE: We don't return the error here because we insert all the tasks
	// without rollbacking the transaction. The errors are returned as part
	// of the (RunResult) response on the individual tasks.

	// Tasks targeting machines.
	for _, machineTask := range target.Machines {
		// Create the task UUID.
		taskUUID, err := internaluuid.NewUUID()
		if err != nil {
			taskResult := operation.MachineTaskResult{
				ReceiverName: machineTask,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("generating task UUID: %w", err),
				},
			}
			result.Machines = append(result.Machines, taskResult)
			continue
		}
		taskResult := s.createMachineTask(ctx, tx, operationUUID, taskUUID.String(), machineTask)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		result.Machines = append(result.Machines, taskResult)
	}

	// Tasks targeting units.
	// Before this, we get all units from the targeted applications, and add
	// them to the originally targeted units.
	totalTargettedUnits := target.Units
	for _, appName := range target.Applications {
		unitsFromApp, err := s.getUnitsForApplication(ctx, tx, appName)
		if err != nil {
			return operation.RunResult{}, errors.Errorf("getting units for application %s: %w", appName, err)
		}
		totalTargettedUnits = append(totalTargettedUnits, unitsFromApp...)
	}

	// Now we can add all the units (from applications or directly from
	// targeted units).
	result.Units = make([]operation.UnitTaskResult, len(totalTargettedUnits))
	for i, targetUnit := range totalTargettedUnits {
		// Create the task UUID.
		taskUUID, err := internaluuid.NewUUID()
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				ReceiverName: targetUnit,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("generating task UUID: %w", err),
				},
			}
			continue
		}
		taskResult := s.createUnitTask(ctx, tx, operationUUID, taskUUID.String(), targetUnit)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		result.Units[i] = taskResult
	}

	return result, nil
}

func (s *State) insertOperation(ctx context.Context, tx *sqlair.TX, args insertOperation) error {
	query := `
INSERT INTO operation (uuid, operation_id, summary, enqueued_at, parallel, execution_group)
VALUES ($insertOperation.*)
`
	stmt, err := s.Prepare(query, args)
	if err != nil {
		return errors.Errorf("preparing insert operation statement: %w", err)
	}

	return tx.Query(ctx, stmt, args).Run()
}

func (s *State) insertOperationParameter(ctx context.Context, tx *sqlair.TX, operationUUID, key, value string) error {
	param := taskParameter{
		OperationUUID: operationUUID,
		Key:           key,
		Value:         value,
	}

	query := `
INSERT INTO operation_parameter (operation_uuid, key, value)
VALUES ($taskParameter.*)
`
	stmt, err := s.Prepare(query, param)
	if err != nil {
		return errors.Errorf("preparing insert operation parameter statement: %w", err)
	}

	return tx.Query(ctx, stmt, param).Run()
}

// insertOperationAction inserts an operation action with a known
// charm UUID.
func (s *State) insertOperationAction(ctx context.Context, tx *sqlair.TX, operationUUID string, charmUUID string, actionName string) error {
	action := insertOperationAction{
		OperationUUID:  operationUUID,
		CharmUUID:      charmUUID,
		CharmActionKey: actionName,
	}

	query := `
INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key)
VALUES ($insertOperationAction.*)
`
	stmt, err := s.Prepare(query, action)
	if err != nil {
		return errors.Errorf("preparing insert operation action statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, action).Run())
}

func (s *State) createMachineTask(
	ctx context.Context,
	tx *sqlair.TX,
	operationUUID string,
	taskUUID string,
	machineName machine.Name,
) operation.MachineTaskResult {
	taskID, err := sequencestate.NextValue(ctx, s, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("generating task ID: %w", err),
			},
		}
	}

	return s.createMachineTaskWithID(ctx, tx, fmt.Sprintf("%d", taskID), taskUUID, operationUUID, machineName)
}

func (s *State) createMachineTaskWithID(ctx context.Context, tx *sqlair.TX, taskID string, taskUUID string, operationUUID string, machineName machine.Name) operation.MachineTaskResult {
	now := time.Now().UTC()

	err := s.insertOperationTask(ctx, tx, taskUUID, operationUUID, taskID, now)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task: %w", err),
			},
		}
	}
	err = s.insertOperationTaskStatus(ctx, tx, taskUUID, corestatus.Pending)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task status: %w", err),
			},
		}
	}

	machineUUID, err := s.getMachineUUID(ctx, tx, machineName)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("getting machine UUID for %s: %w", machineName, err),
			},
		}
	}
	err = s.insertOperationMachineTask(ctx, tx, taskUUID, machineUUID)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("linking task to machine: %w", err),
			},
		}
	}

	return operation.MachineTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:       taskID,
			Enqueued: now,
			Status:   corestatus.Pending,
		},
		ReceiverName: machineName,
	}
}

func (s *State) createUnitTask(ctx context.Context, tx *sqlair.TX, operationUUID string, taskUUID string, unitName coreunit.Name) operation.UnitTaskResult {
	taskID, err := sequencestate.NextValue(ctx, s, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("generating task ID: %w", err),
			},
		}
	}

	return s.createUnitTaskWithID(ctx, tx, fmt.Sprintf("%d", taskID), taskUUID, operationUUID, unitName)
}

func (s *State) createUnitTaskWithID(ctx context.Context, tx *sqlair.TX, taskID string, taskUUID string, operationUUID string, unitName coreunit.Name) operation.UnitTaskResult {
	now := time.Now().UTC()

	err := s.insertOperationTask(ctx, tx, taskUUID, operationUUID, taskID, now)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task: %w", err),
			},
		}
	}

	err = s.insertOperationTaskStatus(ctx, tx, taskUUID, corestatus.Pending)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task status: %w", err),
			},
		}
	}

	err = s.insertOperationUnitTask(ctx, tx, taskUUID, unitName)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("linking task to unit: %w", err),
			},
		}
	}

	return operation.UnitTaskResult{
		TaskInfo: operation.TaskInfo{
			ID:       taskID,
			Enqueued: now,
			Status:   corestatus.Pending,
		},
		ReceiverName: unitName,
	}
}

func (s *State) insertOperationTask(ctx context.Context, tx *sqlair.TX, taskUUID, operationUUID, taskID string, enqueuedAt time.Time) error {
	task := insertOperationTask{
		UUID:          taskUUID,
		OperationUUID: operationUUID,
		TaskID:        taskID,
		EnqueuedAt:    enqueuedAt,
	}

	query := `
INSERT INTO operation_task (uuid, operation_uuid, task_id, enqueued_at)
VALUES ($insertOperationTask.*)
`
	stmt, err := s.Prepare(query, task)
	if err != nil {
		return errors.Errorf("preparing insert operation task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, task).Run())
}

func (s *State) insertOperationTaskStatus(ctx context.Context, tx *sqlair.TX, taskUUID string, status corestatus.Status) error {
	statusValue := insertTaskStatus{
		TaskUUID: taskUUID,
		Status:   status.String(),
	}

	query := `
INSERT INTO operation_task_status (task_uuid, status_id) 
SELECT $insertTaskStatus.task_uuid, id 
FROM operation_task_status_value 
WHERE status = $insertTaskStatus.status`
	stmt, err := s.Prepare(query, statusValue)
	if err != nil {
		return errors.Errorf("preparing insert operation task status statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, statusValue).Run())
}

func (s *State) insertOperationUnitTask(ctx context.Context, tx *sqlair.TX, taskUUID string, unitName coreunit.Name) error {
	unitUUID, err := s.getUnitUUID(ctx, tx, unitName)
	if err != nil {
		return errors.Errorf("getting unit UUID for %s: %w", unitName, err)
	}

	unitTask := insertUnitTask{
		TaskUUID: taskUUID,
		UnitUUID: unitUUID,
	}

	query := `
INSERT INTO operation_unit_task (task_uuid, unit_uuid)
VALUES ($insertUnitTask.*)
`
	stmt, err := s.Prepare(query, insertUnitTask{})
	if err != nil {
		return errors.Errorf("preparing insert operation unit task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, unitTask).Run())
}

func (s *State) insertOperationMachineTask(ctx context.Context, tx *sqlair.TX, taskUUID, machineUUID string) error {
	machineTask := insertMachineTask{
		TaskUUID:    taskUUID,
		MachineUUID: machineUUID,
	}

	query := `
INSERT INTO operation_machine_task (task_uuid, machine_uuid)
VALUES ($insertMachineTask.*)
`
	stmt, err := s.Prepare(query, insertMachineTask{})
	if err != nil {
		return errors.Errorf("preparing insert operation machine task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, machineTask).Run())
}

func (s *State) getAllMachines(ctx context.Context, tx *sqlair.TX) ([]machine.Name, error) {
	query := `
SELECT name AS &nameArg.name 
FROM   machine 
WHERE  life_id = 0`
	stmt, err := s.Prepare(query, nameArg{})
	if err != nil {
		return nil, errors.Errorf("preparing get all machines statement: %w", err)
	}

	var results []nameArg
	err = tx.Query(ctx, stmt).GetAll(&results)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, errors.Errorf("querying all machines: %w", err)
	}

	machines := make([]machine.Name, len(results))
	for i, result := range results {
		machines[i] = machine.Name(result.Name)
	}

	return machines, nil
}

func (s *State) getUnitsForApplication(ctx context.Context, tx *sqlair.TX, appName string) ([]coreunit.Name, error) {
	// We need to alias the nameArg generic struct because we are already using
	// it as input for the query.
	type unitResult nameArg

	ident := nameArg{Name: appName}
	query := `
SELECT u.name AS &unitResult.name
FROM   unit u
JOIN   application a ON u.application_uuid = a.uuid
WHERE  a.name = $nameArg.name AND u.life_id = 0
`

	stmt, err := s.Prepare(query, unitResult{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing get units for application statement: %w", err)
	}

	var results []unitResult
	err = tx.Query(ctx, stmt, ident).GetAll(&results)
	if err != nil {
		return nil, errors.Errorf("querying units for application %s: %w", appName, err)
	}

	units := make([]coreunit.Name, len(results))
	for i, result := range results {
		units[i] = coreunit.Name(result.Name)
	}

	return units, nil
}

func (s *State) getMachineUUID(ctx context.Context, tx *sqlair.TX, machineName machine.Name) (string, error) {
	ident := nameArg{Name: string(machineName)}

	query := `
SELECT uuid AS &uuid.uuid 
FROM   machine 
WHERE  name = $nameArg.name`
	stmt, err := s.Prepare(query, uuid{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get machine UUID statement: %w", err)
	}

	var result uuid
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if err != nil {
		return "", errors.Errorf("querying machine UUID for %s: %w", machineName, err)
	}

	return result.UUID, nil
}

func (s *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (string, error) {
	ident := nameArg{Name: unitName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   unit 
WHERE  name = $nameArg.name`
	stmt, err := s.Prepare(query, uuid{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get unit UUID statement: %w", err)
	}

	var result uuid
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if err != nil {
		return "", errors.Errorf("querying unit UUID for %s: %w", unitName, err)
	}

	return result.UUID, nil
}

func (s *State) getCharmUUIDByUnit(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (string, error) {
	ident := nameArg{Name: unitName.String()}
	query := `
SELECT charm_uuid AS &charmUUIDResult.charm_uuid
FROM   unit
WHERE  unit.name = $nameArg.name`
	stmt, err := s.Prepare(query, charmUUIDResult{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get charm UUID for action statement: %w", err)
	}

	var result charmUUIDResult
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if err != nil {
		return "", errors.Errorf("querying charm UUID for unit %s: %w", unitName, err)
	}

	return result.CharmUUID, nil
}
