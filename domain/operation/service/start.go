// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"context"

	"github.com/juju/juju/core/trace"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/operation"
	"github.com/juju/juju/internal/errors"
	internaluuid "github.com/juju/juju/internal/uuid"
)

// AddExecOperation creates an exec operation with tasks for various machines
// and units, using the provided parameters.
func (s *Service) AddExecOperation(
	ctx context.Context,
	target operation.Receivers,
	args operation.ExecArgs,
) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Initialize the results with proper size upfront
	result := operation.RunResult{}
	result.Units = make([]operation.UnitTaskResult, len(target.LeaderUnit))

	// Initialize a map to store pointers to the result units, indexed by unit name.
	// This is used to consolidate the results from the state layer.
	leaderUnitsByName := make(map[coreunit.Name]*operation.UnitTaskResult)

	// Initialize input unit names to pass to the state layer method.
	leaderUnits := make([]coreunit.Name, 0, len(target.LeaderUnit))

	// Process each leader unit target - need to resolve the actual leader units
	for i, appName := range target.LeaderUnit {
		leaderUnit, err := s.leadershipService.ApplicationLeader(appName)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(appName + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("getting leader unit for %s: %w", appName, err),
				},
			}
			continue
		}

		leaderUnitName, err := coreunit.NewName(leaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(appName + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("parsing unit name for %s: %w", leaderUnit, err),
				},
			}
			continue
		}

		result.Units[i] = operation.UnitTaskResult{
			ReceiverName: leaderUnitName,
			IsLeader:     true,
		}
		leaderUnitsByName[leaderUnitName] = &result.Units[i]
		leaderUnits = append(leaderUnits, leaderUnitName)
	}

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	targetWithResolvedLeaders := operation.ReceiversWithoutLeader{
		Applications: target.Applications,
		Machines:     target.Machines,
		Units:        append(target.Units, leaderUnits...),
	}
	runResult, err := s.st.AddExecOperation(ctx, operationUUID, targetWithResolvedLeaders, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation: %w", err)
	}
	result.OperationID = runResult.OperationID
	// Machine results don't need consolidation because no errors can occur
	// before the state layer insertion method.
	result.Machines = runResult.Machines

	// Consolidate the leader unit results from the state layer with our
	// pre-computed results.
	for _, unitTaskResult := range runResult.Units {
		unitResult, ok := leaderUnitsByName[unitTaskResult.ReceiverName]
		if !ok {
			// This is a regular unit (not a leader), add it directly to the
			// result.
			result.Units = append(result.Units, unitTaskResult)
			continue
		}
		// Update the leader unit result with the actual task info from the
		// state layer.
		unitResult.TaskInfo = unitTaskResult.TaskInfo
	}

	// Mark any missing results (leader units that were expected but not
	// returned by the state layer method).
	for unitName, unitResult := range leaderUnitsByName {
		if unitResult.TaskInfo.ID == "" && unitResult.TaskInfo.Error == nil {
			unitResult.TaskInfo.Error = errors.Errorf("missing result for unit %s", unitName)
		}
	}

	return result, nil
}

// AddExecOperationOnAllMachines creates an exec operation with tasks based on
// the provided parameters on all machines.
func (s *Service) AddExecOperationOnAllMachines(ctx context.Context, args operation.ExecArgs) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	result, err := s.st.AddExecOperationOnAllMachines(ctx, operationUUID, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("starting exec operation on all machines: %w", err)
	}

	return result, nil
}

// AddActionOperation creates an action operation with tasks for various units
// using the provided parameters.
func (s *Service) AddActionOperation(
	ctx context.Context,
	target []operation.ActionReceiver,
	args operation.TaskArgs,
) (operation.RunResult, error) {
	ctx, span := trace.Start(ctx, trace.NameFromFunc())
	defer span.End()

	// Initialize the results with proper size upfront
	result := operation.RunResult{}
	result.Units = make([]operation.UnitTaskResult, len(target))

	// Initialize a map to store pointers to the result units, indexed by unit name.
	// This is used to consolidate the results from the state layer.
	unitResultsByName := make(map[coreunit.Name]*operation.UnitTaskResult)

	// Initialize input unit names to pass to the state layer method.
	targetUnits := make([]coreunit.Name, 0, len(target))

	// Process each target receiver
	for i, unitOrLeader := range target {
		if unitOrLeader.Unit != "" {
			// Direct unit target
			result.Units[i] = operation.UnitTaskResult{
				ReceiverName: unitOrLeader.Unit,
			}
			unitResultsByName[unitOrLeader.Unit] = &result.Units[i]
			targetUnits = append(targetUnits, unitOrLeader.Unit)
			continue
		}

		// Leader unit target - need to resolve the actual leader unit
		leaderUnit, err := s.leadershipService.ApplicationLeader(unitOrLeader.LeaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(unitOrLeader.LeaderUnit + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("getting leader unit for %s: %w", unitOrLeader.LeaderUnit, err),
				},
			}
			continue
		}

		leaderUnitName, err := coreunit.NewName(leaderUnit)
		if err != nil {
			result.Units[i] = operation.UnitTaskResult{
				// Since we haven't gotten the correct leader unit, we need to
				// create an artificial (hard-coded) unit name. This is needed
				// because the API layer consolidates the results based on the
				// receiver name, and it only extracts the application and then
				// concatenates it with "/leader".
				ReceiverName: coreunit.Name(unitOrLeader.LeaderUnit + "/0"),
				IsLeader:     true,
				TaskInfo: operation.TaskInfo{
					Error: errors.Errorf("parsing unit name for %s: %w", leaderUnit, err),
				},
			}
			continue
		}

		result.Units[i] = operation.UnitTaskResult{
			ReceiverName: leaderUnitName,
			IsLeader:     true,
		}
		unitResultsByName[leaderUnitName] = &result.Units[i]
		targetUnits = append(targetUnits, leaderUnitName)
	}

	// If no valid units to process, return early.
	if len(targetUnits) == 0 {
		return result, nil
	}

	operationUUID, err := internaluuid.NewUUID()
	if err != nil {
		return operation.RunResult{}, errors.Errorf("generating operation UUID: %w", err)
	}

	runResult, err := s.st.AddActionOperation(ctx, operationUUID, targetUnits, args)
	if err != nil {
		return operation.RunResult{}, errors.Errorf("adding action operation: %w", err)
	}
	result.OperationID = runResult.OperationID

	// Consolidate the results from the state layer with our pre-computed
	// results.
	for _, unitTaskResult := range runResult.Units {
		unitResult, ok := unitResultsByName[unitTaskResult.ReceiverName]
		if !ok {
			// This should not happen, and we cannot error out.
			continue
		}
		// Update the result with the actual task info from the state layer.
		unitResult.TaskInfo = unitTaskResult.TaskInfo
	}

	// Mark any missing results (units that were expected but not returned by
	// the state layer method).
	for unitName, unitResult := range unitResultsByName {
		if unitResult.TaskInfo.ID == "" && unitResult.TaskInfo.Error == nil {
			unitResult.TaskInfo.Error = errors.Errorf("missing result for unit %s", unitName)
		}
	}

	return result, nil
}
