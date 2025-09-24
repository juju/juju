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
	applicationerrors "github.com/juju/juju/domain/application/errors"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/domain/operation/internal"
	sequencestate "github.com/juju/juju/domain/sequence/state"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddExecOperation creates an exec operation with tasks for various machines
// and units, using the provided parameters.
//
// The following errors may be returned:
// - [applicationerrors.ApplicationNotFound]: If the provided application
// target does not exist.
// - [applicationerrors.UnitNotFound]: If the provided unit target does not
// exist.
// - [machineerrors.MachineNotFound]: If the provided machine target does not
// exist.
func (st *State) AddExecOperation(
	ctx context.Context,
	operationUUID internaluuid.UUID,
	target internal.ReceiversWithResolvedLeaders,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		result, err = st.addExecOperation(ctx, tx, operationUUID.String(), target, args)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("adding exec operation: %w", err)
	}

	return result, nil
}

// AddExecOperationOnAllMachines creates an exec operation with tasks based
// on the provided parameters on all machines.
func (st *State) AddExecOperationOnAllMachines(
	ctx context.Context,
	operationUUID internaluuid.UUID,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	db, err := st.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		machines, err := st.getAllMachines(ctx, tx)
		if err != nil {
			return errors.Capture(err)
		}

		target := internal.ReceiversWithResolvedLeaders{
			Machines: machines,
		}

		result, err = st.addExecOperation(ctx, tx, operationUUID.String(), target, args)
		return errors.Capture(err)
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("adding exec operation on all machines: %w", err)
	}

	return result, nil
}

// AddActionOperation creates an action operation with tasks for various
// units using the provided parameters.
//
// The following errors may be returned:
// - [applicationerrors.UnitNotFound]: If the provided unit target does not
// exist.
func (st *State) AddActionOperation(ctx context.Context,
	operationUUID internaluuid.UUID,
	targetUnits []coreunit.Name,
	args operation.TaskArgs,
) (operation.RunResult, error) {
	// We need at least one unit target task.
	if len(targetUnits) == 0 {
		return operation.RunResult{}, errors.Errorf("no target units provided")
	}

	db, err := st.DB(ctx)
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var err error

		// Generate the operation ID.
		operationID, err := sequencestate.NextValue(ctx, st, tx, operation.OperationSequenceNamespace)
		if err != nil {
			return errors.Errorf("generating operation ID: %w", err)
		}
		result.OperationID = strconv.FormatUint(operationID, 10)

		// Insert the operation first.
		err = st.insertOperation(ctx, tx, insertOperation{
			UUID:           operationUUID.String(),
			OperationID:    strconv.FormatUint(operationID, 10),
			Summary:        fmt.Sprintf("action %q", args.ActionName),
			EnqueuedAt:     time.Now().UTC(),
			Parallel:       args.IsParallel,
			ExecutionGroup: args.ExecutionGroup,
		})
		if err != nil {
			return errors.Errorf("inserting operation: %w", err)
		}

		// Here we assume that all target units share the same application.
		charmUUID, err := st.getCharmUUIDByApplication(ctx, tx, targetUnits[0].Application())
		if err != nil {
			return errors.Errorf("getting charm UUID for application %q: %w", targetUnits[0].Application(), err)
		}

		err = st.insertOperationAction(ctx, tx, operationUUID.String(), charmUUID, args.ActionName)
		if err != nil {
			return errors.Errorf("inserting operation action: %w", err)
		}

		// Insert operation parameters.
		for key, value := range args.Parameters {
			err = st.insertOperationParameter(ctx, tx, operationUUID.String(), key, fmt.Sprintf("%v", value))
			if err != nil {
				return errors.Errorf("inserting parameter %q: %w", key, err)
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
			result.Units[i] = st.addUnitTask(ctx, tx, operationUUID.String(), taskUUID.String(), targetUnit)
		}
		return nil
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("adding action operation: %w", err)
	}

	return result, nil
}

// // addMachineExecTargets inserts a list of machine tasks for the provided target
// // and operation UUID.
// func (st *State) addMachineExecTargets(
// 	ctx context.Context,
// 	tx *sqlair.TX,
// 	operationUUID string,
// 	machineTargets []machine.UUID,
// ) []operation.MachineTaskResult {
// 	for _, machineTask := range machineTargets {
// 		// Create the task UUID.
// 		taskUUID, err := internaluuid.NewUUID()
// 		if err != nil {
// 			taskResult := operation.MachineTaskResult{
// 				ReceiverName: machineTask,
// 				TaskInfo: operation.TaskInfo{
// 					Error: errors.Errorf("generating task UUID: %w", err),
// 				},
// 			}
// 			result.Machines = append(result.Machines, taskResult)
// 			continue
// 		}
// 		taskResult := st.addMachineTask(ctx, tx, operationUUID, taskUUID.String(), machineTask)
// 		taskResult.IsParallel = args.Parallel
// 		taskResult.ExecutionGroup = &args.ExecutionGroup
// 		result.Machines = append(result.Machines, taskResult)
// 	}
// }

// addExecOperation creates the exec operation for the provided receivers.
func (st *State) addExecOperation(
	ctx context.Context,
	tx *sqlair.TX,
	operationUUID string,
	target internal.ReceiversWithResolvedLeaders,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	// Generate the operation ID.
	operationID, err := sequencestate.NextValue(ctx, st, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation ID: %w", err)
	}

	now := time.Now().UTC()
	// Insert the operation first.
	err = st.insertOperation(ctx, tx, insertOperation{
		UUID:           operationUUID,
		OperationID:    strconv.FormatUint(operationID, 10),
		Summary:        fmt.Sprintf("exec %q", args.Command),
		EnqueuedAt:     now,
		Parallel:       args.Parallel,
		ExecutionGroup: args.ExecutionGroup,
	})
	if err != nil {
		return operation.RunResult{}, errors.Errorf("inserting operation: %w", err)
	}

	// Exec operations have command and timeout parameters.
	err = st.addExecParameters(ctx, tx, operationUUID, args.Command, args.Timeout.String())
	if err != nil {
		return operation.RunResult{}, errors.Capture(err)
	}

	var result operation.RunResult
	result.OperationID = strconv.FormatUint(operationID, 10)

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
		taskResult := st.addMachineTask(ctx, tx, operationUUID, taskUUID.String(), machineTask)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		result.Machines = append(result.Machines, taskResult)
	}

	// Tasks targeting units.
	for _, unitTask := range target.Units {
		// Create the task UUID.
		taskUUID, err := internaluuid.NewUUID()
		if err != nil {
			taskResult := operation.UnitTaskResult{
				ReceiverName: unitTask,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("generating task UUID: %w", err),
				},
			}
			result.Units = append(result.Units, taskResult)
			continue
		}
		taskResult := st.addUnitTask(ctx, tx, operationUUID, taskUUID.String(), unitTask)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		result.Units = append(result.Units, taskResult)
	}

	// Now we get all units from the targeted applications, and append them to
	// the originally targeted units.
	var applicationTargetUnits []coreunit.Name
	if len(target.Applications) != 0 {
		applicationTargetUnits, err = st.getUnitsForApplications(ctx, tx, target.Applications)
		if err != nil {
			return operation.RunResult{}, errors.Errorf("getting units for applications %v: %w", target.Applications, err)
		}
	}
	// Insert the units from the provided applications.
	for _, unitTask := range applicationTargetUnits {
		// Create the task UUID.
		taskUUID, err := internaluuid.NewUUID()
		if err != nil {
			taskResult := operation.UnitTaskResult{
				ReceiverName: unitTask,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("generating task UUID: %w", err),
				},
			}
			result.Units = append(result.Units, taskResult)
			continue
		}
		taskResult := st.addUnitTask(ctx, tx, operationUUID, taskUUID.String(), unitTask)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		result.Units = append(result.Units, taskResult)
	}

	// Now we can add the leader units.
	for _, unitTask := range target.LeaderUnits {
		// Create the task UUID.
		taskUUID, err := internaluuid.NewUUID()
		if err != nil {
			taskResult := operation.UnitTaskResult{
				ReceiverName: unitTask,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("generating task UUID: %w", err),
				},
			}
			result.Units = append(result.Units, taskResult)
			continue
		}
		taskResult := st.addUnitTask(ctx, tx, operationUUID, taskUUID.String(), unitTask)
		taskResult.IsParallel = args.Parallel
		taskResult.ExecutionGroup = &args.ExecutionGroup
		// This is a leader unit, so we need to set the leader flag for proper
		// consolidation of the results.
		taskResult.IsLeader = true
		result.Units = append(result.Units, taskResult)
	}

	return result, nil
}

// addExecParameters inserts the exec operation parameters, which must
// contain the actual command and a timeout.
func (st *State) addExecParameters(ctx context.Context, tx *sqlair.TX, operationUUID string, command string, timeout string) error {
	err := st.insertOperationParameter(ctx, tx, operationUUID, "command", command)
	if err != nil {
		return errors.Errorf("inserting command parameter: %w", err)
	}
	err = st.insertOperationParameter(ctx, tx, operationUUID, "timeout", timeout)
	if err != nil {
		return errors.Errorf("inserting timeout parameter: %w", err)
	}
	return nil
}

func (st *State) insertOperation(ctx context.Context, tx *sqlair.TX, args insertOperation) error {
	query := `
INSERT INTO operation (uuid, operation_id, summary, enqueued_at, parallel, execution_group)
VALUES ($insertOperation.*)
`
	stmt, err := st.Prepare(query, args)
	if err != nil {
		return errors.Errorf("preparing insert operation statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, args).Run())
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

	return errors.Capture(tx.Query(ctx, stmt, param).Run())
}

// insertOperationAction inserts an operation action with a known
// charm UUID.
func (st *State) insertOperationAction(ctx context.Context, tx *sqlair.TX, operationUUID string, charmUUID string, actionName string) error {
	action := insertOperationAction{
		OperationUUID:  operationUUID,
		CharmUUID:      charmUUID,
		CharmActionKey: actionName,
	}

	query := `
INSERT INTO operation_action (operation_uuid, charm_uuid, charm_action_key)
VALUES ($insertOperationAction.*)
`
	stmt, err := st.Prepare(query, action)
	if err != nil {
		return errors.Errorf("preparing insert operation action statement: %w", err)
	}

	err = tx.Query(ctx, stmt, action).Run()
	if err != nil {
		// We know that we can have a FK error here if the charm action (
		// charm_action_key) does not exist for the provided charm, so we return
		// a user error.
		return errors.Errorf("inserting action %q for charm %q and operation %q", actionName, charmUUID, operationUUID)
	}
	return nil
}

func (st *State) addMachineTask(
	ctx context.Context,
	tx *sqlair.TX,
	operationUUID string,
	taskUUID string,
	machineName machine.Name,
) operation.MachineTaskResult {
	taskID, err := sequencestate.NextValue(ctx, st, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("generating task ID: %w", err),
			},
		}
	}

	return st.addMachineTaskWithID(ctx, tx, strconv.FormatUint(taskID, 10), taskUUID, operationUUID, machineName)
}

func (st *State) addMachineTaskWithID(ctx context.Context, tx *sqlair.TX, taskID string, taskUUID string, operationUUID string, machineName machine.Name) operation.MachineTaskResult {
	now := time.Now().UTC()

	err := st.insertOperationTask(ctx, tx, taskUUID, operationUUID, taskID, now)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task: %w", err),
			},
		}
	}
	err = st.insertOperationTaskStatus(ctx, tx, taskUUID, corestatus.Pending)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task status: %w", err),
			},
		}
	}

	machineUUID, err := st.getMachineUUID(ctx, tx, machineName)
	if err != nil {
		return operation.MachineTaskResult{
			ReceiverName: machineName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("getting machine UUID for %q: %w", machineName, err),
			},
		}
	}
	err = st.insertOperationMachineTask(ctx, tx, taskUUID, machineUUID)
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

func (st *State) addUnitTask(ctx context.Context, tx *sqlair.TX, operationUUID string, taskUUID string, unitName coreunit.Name) operation.UnitTaskResult {
	taskID, err := sequencestate.NextValue(ctx, st, tx, operation.OperationSequenceNamespace)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("generating task ID: %w", err),
			},
		}
	}

	return st.addUnitTaskWithID(ctx, tx, strconv.FormatUint(taskID, 10), taskUUID, operationUUID, unitName)
}

func (st *State) addUnitTaskWithID(ctx context.Context, tx *sqlair.TX, taskID string, taskUUID string, operationUUID string, unitName coreunit.Name) operation.UnitTaskResult {
	now := time.Now().UTC()

	err := st.insertOperationTask(ctx, tx, taskUUID, operationUUID, taskID, now)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task: %w", err),
			},
		}
	}

	err = st.insertOperationTaskStatus(ctx, tx, taskUUID, corestatus.Pending)
	if err != nil {
		return operation.UnitTaskResult{
			ReceiverName: unitName,
			TaskInfo: operation.TaskInfo{
				Error: errors.Errorf("inserting operation task status: %w", err),
			},
		}
	}

	err = st.insertOperationUnitTask(ctx, tx, taskUUID, unitName)
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

func (st *State) insertOperationTask(ctx context.Context, tx *sqlair.TX, taskUUID, operationUUID, taskID string, enqueuedAt time.Time) error {
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
	stmt, err := st.Prepare(query, task)
	if err != nil {
		return errors.Errorf("preparing insert operation task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, task).Run())
}

func (st *State) insertOperationTaskStatus(ctx context.Context, tx *sqlair.TX, taskUUID string, status corestatus.Status) error {
	statusValue := insertTaskStatus{
		TaskUUID:  taskUUID,
		Status:    string(status),
		UpdatedAt: time.Now().UTC(),
	}

	query := `
INSERT INTO operation_task_status (task_uuid, status_id, updated_at) 
SELECT $insertTaskStatus.task_uuid, id, $insertTaskStatus.updated_at
FROM operation_task_status_value 
WHERE status = $insertTaskStatus.status`
	stmt, err := st.Prepare(query, statusValue)
	if err != nil {
		return errors.Errorf("preparing insert operation task status statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, statusValue).Run())
}

func (st *State) insertOperationUnitTask(ctx context.Context, tx *sqlair.TX, taskUUID string, unitName coreunit.Name) error {
	unitUUID, err := st.getUnitUUID(ctx, tx, unitName)
	if err != nil {
		return errors.Errorf("getting unit UUID for %q: %w", unitName, err)
	}

	unitTask := insertUnitTask{
		TaskUUID: taskUUID,
		UnitUUID: unitUUID,
	}

	query := `
INSERT INTO operation_unit_task (task_uuid, unit_uuid)
VALUES ($insertUnitTask.*)
`
	stmt, err := st.Prepare(query, insertUnitTask{})
	if err != nil {
		return errors.Errorf("preparing insert operation unit task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, unitTask).Run())
}

func (st *State) insertOperationMachineTask(ctx context.Context, tx *sqlair.TX, taskUUID, machineUUID string) error {
	machineTask := insertMachineTask{
		TaskUUID:    taskUUID,
		MachineUUID: machineUUID,
	}

	query := `
INSERT INTO operation_machine_task (task_uuid, machine_uuid)
VALUES ($insertMachineTask.*)
`
	stmt, err := st.Prepare(query, insertMachineTask{})
	if err != nil {
		return errors.Errorf("preparing insert operation machine task statement: %w", err)
	}

	return errors.Capture(tx.Query(ctx, stmt, machineTask).Run())
}

func (st *State) getAllMachines(ctx context.Context, tx *sqlair.TX) ([]machine.Name, error) {
	query := `
SELECT name AS &nameArg.name 
FROM   machine 
WHERE  life_id = 0`
	stmt, err := st.Prepare(query, nameArg{})
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

func (st *State) getUnitsForApplications(ctx context.Context, tx *sqlair.TX, appNames []string) ([]coreunit.Name, error) {
	// We need to alias the nameArg generic struct because we are already using
	// it as input for the query.
	type unitResult nameArg

	type names []string
	query := `
SELECT u.name AS &unitResult.name
FROM   unit u
JOIN   application a ON u.application_uuid = a.uuid
WHERE  a.name IN ($names[:])
`
	ident := names(appNames)

	stmt, err := st.Prepare(query, unitResult{}, ident)
	if err != nil {
		return nil, errors.Errorf("preparing get units for application statement: %w", err)
	}

	var results []unitResult
	err = tx.Query(ctx, stmt, ident).GetAll(&results)
	if errors.Is(err, sqlair.ErrNoRows) {
		return nil, errors.Errorf("no application found").Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return nil, errors.Errorf("querying units for applications %v: %w", appNames, err)
	}

	units := make([]coreunit.Name, len(results))
	for i, result := range results {
		units[i] = coreunit.Name(result.Name)
	}

	return units, nil
}

func (st *State) getMachineUUID(ctx context.Context, tx *sqlair.TX, machineName machine.Name) (string, error) {
	ident := nameArg{Name: string(machineName)}

	query := `
SELECT uuid AS &uuid.uuid 
FROM   machine 
WHERE  name = $nameArg.name`
	stmt, err := st.Prepare(query, uuid{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get machine UUID statement: %w", err)
	}

	var result uuid
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("machine %q not found", machineName).Add(machineerrors.MachineNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying machine UUID for %q: %w", machineName, err)
	}

	return result.UUID, nil
}

func (st *State) getUnitUUID(ctx context.Context, tx *sqlair.TX, unitName coreunit.Name) (string, error) {
	ident := nameArg{Name: unitName.String()}
	query := `
SELECT uuid AS &uuid.uuid 
FROM   unit 
WHERE  name = $nameArg.name`
	stmt, err := st.Prepare(query, uuid{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get unit UUID statement: %w", err)
	}

	var result uuid
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("unit %q not found", unitName).Add(applicationerrors.UnitNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying unit UUID for %q: %w", unitName, err)
	}

	return result.UUID, nil
}

func (st *State) getCharmUUIDByApplication(ctx context.Context, tx *sqlair.TX, appName string) (string, error) {
	ident := nameArg{Name: appName}
	query := `
SELECT charm_uuid AS &charmUUIDResult.charm_uuid
FROM   application
WHERE  application.name = $nameArg.name`
	stmt, err := st.Prepare(query, charmUUIDResult{}, ident)
	if err != nil {
		return "", errors.Errorf("preparing get charm UUID for action statement: %w", err)
	}

	var result charmUUIDResult
	err = tx.Query(ctx, stmt, ident).Get(&result)
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.Errorf("application %q not found", appName).Add(applicationerrors.ApplicationNotFound)
	} else if err != nil {
		return "", errors.Errorf("querying charm UUID for application %q: %w", appName, err)
	}

	return result.CharmUUID, nil
}
